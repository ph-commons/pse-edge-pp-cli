// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openPSETestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.EnsurePSEEdgeTables(context.Background()); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	return s
}

func TestEnsurePSEEdgeTablesIdempotent(t *testing.T) {
	s := openPSETestStore(t)
	// Second call must be a no-op, not an error.
	if err := s.EnsurePSEEdgeTables(context.Background()); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	for _, table := range []string{"pse_companies", "pse_eod_prices", "pse_index_snapshots", "pse_disclosures"} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing (n=%d err=%v)", table, n, err)
		}
	}
}

func TestUpsertPSECompaniesAndLookup(t *testing.T) {
	s := openPSETestStore(t)
	ctx := context.Background()
	rows := []PSECompanyRow{
		{CmpyID: 633, SecurityID: 628, Symbol: "GTCAP", Name: "GT Capital Holdings, Inc."},
		{CmpyID: 34, SecurityID: 320, Symbol: "AT", Name: "Atlas Consolidated Mining and Development Corporation"},
	}
	if err := s.UpsertPSECompanies(ctx, rows); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Idempotent re-upsert with a changed name must update, not duplicate.
	rows[0].Name = "GT Capital Holdings"
	if err := s.UpsertPSECompanies(ctx, rows); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_companies`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("company count = %d (err=%v), want 2", n, err)
	}
	got, err := s.LookupPSECompanyBySymbol(ctx, "GTCAP")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.CmpyID != 633 || got.SecurityID != 628 || got.Name != "GT Capital Holdings" {
		t.Errorf("lookup GTCAP = %+v", got)
	}
	if _, err := s.LookupPSECompanyBySymbol(ctx, "NOPE"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing symbol should be sql.ErrNoRows, got %v", err)
	}
	syms, err := s.ListPSECompanySymbols(ctx)
	if err != nil || len(syms) != 2 || syms[0] != "AT" {
		t.Errorf("ListPSECompanySymbols = %v (err=%v)", syms, err)
	}
}

func TestUpsertPSEEODPrices(t *testing.T) {
	s := openPSETestStore(t)
	ctx := context.Background()
	rows := []PSEEODRow{
		{Symbol: "AT", TradingDate: "2026-07-13", Open: 8.3, High: 8.3, Low: 7.98, Close: 8.08, Value: 1.1852847e7, Source: "edge"},
		{Symbol: "AT", TradingDate: "2026-07-14", Open: 8.1, High: 8.38, Low: 8.1, Close: 8.29, Value: 9651781, Source: "edge"},
	}
	if err := s.UpsertPSEEODPrices(ctx, rows); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Re-upsert with corrected close: updates in place.
	rows[0].Close = 8.10
	if err := s.UpsertPSEEODPrices(ctx, rows); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_eod_prices WHERE symbol='AT'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("row count = %d (err=%v), want 2", n, err)
	}
	var closeV float64
	var volume sql.NullFloat64
	var source string
	if err := s.DB().QueryRow(`SELECT close, volume, source FROM pse_eod_prices WHERE symbol='AT' AND trading_date='2026-07-13'`).Scan(&closeV, &volume, &source); err != nil {
		t.Fatalf("select: %v", err)
	}
	if closeV != 8.10 || source != "edge" {
		t.Errorf("close=%v source=%q", closeV, source)
	}
	if volume.Valid {
		t.Errorf("volume should be NULL when unset, got %v", volume.Float64)
	}
}

func TestUpsertPSEIndexSnapshots(t *testing.T) {
	s := openPSETestStore(t)
	ctx := context.Background()
	pct := 0.54
	change := 33.89
	adv, dec, unch, trades := int64(94), int64(75), int64(39), int64(69416)
	vol, val := 4.48077665e8, 4.81313904736e9
	rows := []PSEIndexSnapshotRow{
		{IndexCode: "PSEI", TradingDate: "2026-07-27", Value: 6314.9, Change: &change, PctChange: &pct,
			Advances: &adv, Declines: &dec, Unchanged: &unch, TotalVolume: &vol, TotalValue: &val, TotalTrades: &trades, Source: "edge"},
		// Backfill-shaped row: close-only, change/breadth fields NULL.
		{IndexCode: "PSEI", TradingDate: "2026-07-24", Value: 6281.01, Source: "edge"},
	}
	if err := s.UpsertPSEIndexSnapshots(ctx, rows); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Idempotent backfill re-run.
	if err := s.UpsertPSEIndexSnapshots(ctx, rows); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_index_snapshots WHERE index_code='PSEI'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("row count = %d (err=%v), want 2", n, err)
	}
	var advances sql.NullInt64
	if err := s.DB().QueryRow(`SELECT advances FROM pse_index_snapshots WHERE trading_date='2026-07-24'`).Scan(&advances); err != nil {
		t.Fatalf("select: %v", err)
	}
	if advances.Valid {
		t.Errorf("backfill row advances should be NULL, got %v", advances.Int64)
	}
	if err := s.DB().QueryRow(`SELECT advances FROM pse_index_snapshots WHERE trading_date='2026-07-27'`).Scan(&advances); err != nil {
		t.Fatalf("select: %v", err)
	}
	if !advances.Valid || advances.Int64 != 94 {
		t.Errorf("snapshot advances = %v, want 94", advances)
	}
}

// Regression for the backfill-clobber bug: the close-only PSEi series
// backfill re-runs on every sync; its NULL change/breadth/totals must
// NEVER overwrite a breadth-rich post-close snapshot for the same date,
// while its (possibly corrected) close value still updates.
func TestUpsertPSEIndexSnapshotsBackfillNeverClobbersBreadth(t *testing.T) {
	s := openPSETestStore(t)
	ctx := context.Background()

	change := 33.89
	pct := 0.54
	adv, dec, unch, trades := int64(94), int64(75), int64(39), int64(69416)
	vol, val := 4.48077665e8, 4.81313904736e9
	rich := PSEIndexSnapshotRow{
		IndexCode: "PSEI", TradingDate: "2026-07-27", Value: 6314.9, Change: &change, PctChange: &pct,
		Advances: &adv, Declines: &dec, Unchanged: &unch,
		TotalVolume: &vol, TotalValue: &val, TotalTrades: &trades, Source: "edge",
	}
	if err := s.UpsertPSEIndexSnapshots(ctx, []PSEIndexSnapshotRow{rich}); err != nil {
		t.Fatalf("upsert rich row: %v", err)
	}

	// Same-date close-only backfill row (nil change/breadth) with an
	// updated value.
	backfill := PSEIndexSnapshotRow{IndexCode: "PSEI", TradingDate: "2026-07-27", Value: 6300.0, Source: "edge"}
	if err := s.UpsertPSEIndexSnapshots(ctx, []PSEIndexSnapshotRow{backfill}); err != nil {
		t.Fatalf("upsert backfill row: %v", err)
	}

	var value float64
	var changeCol, pctCol, volCol, valCol sql.NullFloat64
	var advCol, decCol, unchCol, tradesCol sql.NullInt64
	err := s.DB().QueryRow(
		`SELECT value, change, pct_change, advances, declines, unchanged, total_volume, total_value, total_trades
		 FROM pse_index_snapshots WHERE index_code='PSEI' AND trading_date='2026-07-27'`,
	).Scan(&value, &changeCol, &pctCol, &advCol, &decCol, &unchCol, &volCol, &valCol, &tradesCol)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if value != 6300.0 {
		t.Errorf("value = %v, want backfill's 6300.0 (value must update)", value)
	}
	if !changeCol.Valid || changeCol.Float64 != 33.89 {
		t.Errorf("change = %+v, want preserved 33.89", changeCol)
	}
	if !pctCol.Valid || pctCol.Float64 != 0.54 {
		t.Errorf("pct_change = %+v, want preserved 0.54", pctCol)
	}
	if !advCol.Valid || advCol.Int64 != 94 || !decCol.Valid || decCol.Int64 != 75 || !unchCol.Valid || unchCol.Int64 != 39 {
		t.Errorf("breadth = adv %+v dec %+v unch %+v, want preserved 94/75/39", advCol, decCol, unchCol)
	}
	if !volCol.Valid || volCol.Float64 != 4.48077665e8 || !valCol.Valid || valCol.Float64 != 4.81313904736e9 || !tradesCol.Valid || tradesCol.Int64 != 69416 {
		t.Errorf("totals = vol %+v val %+v trades %+v, want preserved", volCol, valCol, tradesCol)
	}
}
