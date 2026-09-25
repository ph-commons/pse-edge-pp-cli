// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openQuotationStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEnsureQuotationReportTablesAreSeparate(t *testing.T) {
	s := openQuotationStore(t)
	ctx := context.Background()
	if err := s.EnsurePSEEdgeTables(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pse_quotation_revisions", "pse_quotation_rows", "pse_quotation_ambiguous"} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s was created by EnsurePSEEdgeTables", name)
		}
	}
	if err := s.EnsureQuotationReportTables(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureQuotationReportTables(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pse_quotation_revisions", "pse_quotation_rows", "pse_quotation_ambiguous"} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s count=%d err=%v", name, n, err)
		}
	}
}

func TestPersistAndQueryQuotationRevision(t *testing.T) {
	s := openQuotationStore(t)
	ctx := context.Background()
	if err := s.EnsurePSEEdgeTables(ctx); err != nil {
		t.Fatal(err)
	}
	closeV := 19.98
	volume := 150000.0
	net := -12345.5
	in := QuotationRevisionInput{
		SessionDate: "2026-09-24", SourceSHA256: "abc", SourceURL: "https://documents.pse.com.ph/a.pdf",
		ByteLength: 12, AcquiredAt: "2026-09-24T12:00:00Z", PDFPath: "a.pdf",
		ListingURL: "https://www.pse.com.ph/market-report/", MatchedTitle: "End of Day Quotes",
		Rows: []QuotationStoredRow{
			{Symbol: "AT", IssueName: "ATLAS", RowLocator: "r1", Page: 1, Close: &closeV, Volume: &volume, NetForeign: &net, FieldStatus: map[string]string{"close": "ok", "volume": "ok", "net_foreign": "ok"}},
			{Symbol: "DHI", IssueName: "DOMINION", RowLocator: "r2", Page: 1, FieldStatus: map[string]string{"close": "reported_dash"}},
		},
		Ambiguous: []QuotationAmbiguousRow{{Symbol: "DUP", Locators: []string{"p1", "p2"}}},
	}
	if err := s.PersistQuotationRevision(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := s.PersistQuotationRevision(ctx, in); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_quotation_rows WHERE source_sha256='abc'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("row count=%d err=%v", n, err)
	}
	var eod int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_eod_prices`).Scan(&eod); err != nil || eod != 0 {
		t.Fatalf("pse_eod_prices=%d err=%v", eod, err)
	}
	var dash sql.NullFloat64
	if err := s.DB().QueryRow(`SELECT close FROM pse_quotation_rows WHERE symbol='DHI'`).Scan(&dash); err != nil {
		t.Fatal(err)
	}
	if dash.Valid {
		t.Fatalf("dash close stored as %v", dash.Float64)
	}
	got, err := s.QueryQuotationRevisions(ctx, QuotationQuery{From: "2026-09-24", To: "2026-09-24"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Current || len(got[0].Rows) != 2 {
		t.Fatalf("query=%+v", got)
	}
	by := map[string]QuotationStoredRow{}
	for _, row := range got[0].Rows {
		by[row.Symbol] = row
	}
	if by["AT"].Close == nil || *by["AT"].Close != 19.98 || by["AT"].NetForeign == nil || *by["AT"].NetForeign != -12345.5 {
		t.Fatalf("AT=%+v", by["AT"])
	}
	if by["AT"].FieldStatus["close"] != "ok" || by["DHI"].Close != nil || by["DHI"].FieldStatus["close"] != "reported_dash" {
		t.Fatalf("rows=%+v %+v", by["AT"], by["DHI"])
	}
	if len(got[0].Ambiguous) != 1 || got[0].Ambiguous[0].Symbol != "DUP" {
		t.Fatalf("ambiguous=%+v", got[0].Ambiguous)
	}
}

func TestPersistQuotationRevisionSecondSHA(t *testing.T) {
	s := openQuotationStore(t)
	ctx := context.Background()
	firstClose := 1.0
	secondClose := 3.0
	first := QuotationRevisionInput{
		SessionDate: "2026-09-24", SourceSHA256: "aaa", SourceURL: "https://documents.pse.com.ph/a.pdf",
		AcquiredAt: "2026-09-24T12:00:00Z", PDFPath: "a.pdf",
		Rows: []QuotationStoredRow{{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &firstClose, FieldStatus: map[string]string{"close": "ok"}}},
	}
	if err := s.PersistQuotationRevision(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SourceSHA256 = "bbb"
	second.Rows = []QuotationStoredRow{{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &secondClose, FieldStatus: map[string]string{"close": "ok"}}}
	if err := s.PersistQuotationRevision(ctx, second); err != nil {
		t.Fatal(err)
	}
	var complete, current int
	if err := s.DB().QueryRow(`SELECT complete, is_current FROM pse_quotation_revisions WHERE source_sha256='aaa'`).Scan(&complete, &current); err != nil {
		t.Fatal(err)
	}
	if complete != 1 || current != 0 {
		t.Fatalf("aaa complete=%d current=%d", complete, current)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_quotation_rows WHERE source_sha256='aaa'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("aaa rows=%d err=%v", n, err)
	}
	currentRows, err := s.QueryQuotationRevisions(ctx, QuotationQuery{From: "2026-09-24", To: "2026-09-24"})
	if err != nil || len(currentRows) != 1 || currentRows[0].SourceSHA256 != "bbb" {
		t.Fatalf("current=%+v err=%v", currentRows, err)
	}
	old, err := s.QueryQuotationRevisions(ctx, QuotationQuery{From: "2026-09-24", To: "2026-09-24", SHA: "aaa"})
	if err != nil || len(old) != 1 || old[0].Current || old[0].Rows[0].Close == nil || *old[0].Rows[0].Close != 1 {
		t.Fatalf("old=%+v err=%v", old, err)
	}
	if _, err := s.QueryQuotationRevisions(ctx, QuotationQuery{From: "2026-09-24", To: "2026-09-24", SHA: "missing"}); !errors.Is(err, ErrQuotationRevisionNotFound) {
		t.Fatalf("missing sha err=%v", err)
	}
}

func TestPersistQuotationRevisionRollsBack(t *testing.T) {
	s := openQuotationStore(t)
	ctx := context.Background()
	in := QuotationRevisionInput{
		SessionDate: "2026-09-24", SourceSHA256: "abc", SourceURL: "https://documents.pse.com.ph/a.pdf",
		AcquiredAt: "2026-09-24T12:00:00Z", PDFPath: "a.pdf",
		Rows: []QuotationStoredRow{{Symbol: "AT", RowLocator: "r1", Page: 1, FieldStatus: map[string]string{"close": "reported_dash"}}},
	}
	err := s.persistQuotationRevision(ctx, in, func() error { return errors.New("boom") })
	if err == nil {
		t.Fatal("expected rollback error")
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pse_quotation_revisions WHERE source_sha256='abc' AND complete=1`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("complete rows=%d err=%v", n, err)
	}
	if _, err := s.QueryQuotationRevisions(ctx, QuotationQuery{From: "2026-09-24", To: "2026-09-24", SHA: "abc"}); !errors.Is(err, ErrQuotationRevisionNotFound) {
		t.Fatalf("query err=%v", err)
	}
}
