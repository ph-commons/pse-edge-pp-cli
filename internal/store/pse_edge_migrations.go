// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored PSE Edge domain tables. Lazily created (CREATE TABLE IF
// NOT EXISTS) by the commands that touch them, so the generated migrate()
// path and StoreSchemaVersion stay untouched — older binaries opening the
// same database simply ignore these tables.
//
// Schema is the BUILD-CONTEXT red-team contract for THIS REPO's commands
// (history/drift/breadth/movers/deadlines). It is NOT a public downstream
// ABI — external pipelines should use `export eod|index|companies-local`
// (see docs/downstream-integration.md, issue #9). Internal tables:
//   - pse_companies      registry (cmpy_id PK, security_id, symbol UNIQUE)
//   - pse_eod_prices      final per-session bars, PK(symbol, trading_date)
//   - pse_index_snapshots per-index per-session readings + breadth fields
//   - pse_disclosures     disclosure headers keyed by edge_no

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

var pseEdgeMigrations = []string{
	`CREATE TABLE IF NOT EXISTS pse_companies (
		cmpy_id INTEGER PRIMARY KEY,
		security_id INTEGER,
		symbol TEXT UNIQUE,
		name TEXT,
		etf INTEGER,
		synced_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS pse_eod_prices (
		symbol TEXT,
		trading_date TEXT,
		open REAL,
		high REAL,
		low REAL,
		close REAL,
		value REAL,
		volume REAL NULL,
		source TEXT,
		PRIMARY KEY(symbol, trading_date)
	)`,
	`CREATE TABLE IF NOT EXISTS pse_index_snapshots (
		index_code TEXT,
		trading_date TEXT,
		value REAL,
		change REAL,
		pct_change REAL NULL,
		advances INTEGER NULL,
		declines INTEGER NULL,
		unchanged INTEGER NULL,
		total_volume REAL NULL,
		total_value REAL NULL,
		total_trades INTEGER NULL,
		source TEXT,
		PRIMARY KEY(index_code, trading_date)
	)`,
	`CREATE TABLE IF NOT EXISTS pse_disclosures (
		edge_no TEXT PRIMARY KEY,
		cmpy_id INTEGER,
		symbol TEXT,
		template TEXT,
		title TEXT,
		disclosed_at TEXT,
		synced_at TEXT
	)`,
}

// EnsurePSEEdgeTables lazily creates the hand-authored PSE Edge tables.
// Idempotent; safe to call from every command that reads or writes them.
func (s *Store) EnsurePSEEdgeTables(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for _, m := range pseEdgeMigrations {
		if _, err := s.db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("creating pse-edge table: %w", err)
		}
	}
	return nil
}

// PSECompanyRow is one registry row destined for pse_companies.
type PSECompanyRow struct {
	CmpyID     int
	SecurityID int
	Symbol     string
	Name       string
	ETF        bool
}

// UpsertPSECompanies inserts or refreshes registry rows in one
// transaction. synced_at is stamped UTC RFC3339 at write time.
func (s *Store) UpsertPSECompanies(ctx context.Context, rows []PSECompanyRow) error {
	if len(rows) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range rows {
		etf := 0
		if r.ETF {
			etf = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_companies (cmpy_id, security_id, symbol, name, etf, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(cmpy_id) DO UPDATE SET
			   security_id = excluded.security_id,
			   symbol = excluded.symbol,
			   name = excluded.name,
			   etf = excluded.etf,
			   synced_at = excluded.synced_at`,
			r.CmpyID, r.SecurityID, r.Symbol, r.Name, etf, now,
		); err != nil {
			return fmt.Errorf("upsert pse_companies %s: %w", r.Symbol, err)
		}
	}
	return tx.Commit()
}

// LookupPSECompanyBySymbol resolves a ticker to its registry row. Returns
// sql.ErrNoRows (wrapped) when the symbol is not in the local registry.
func (s *Store) LookupPSECompanyBySymbol(ctx context.Context, symbol string) (*PSECompanyRow, error) {
	var r PSECompanyRow
	var etf sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT cmpy_id, security_id, symbol, name, COALESCE(etf, 0) FROM pse_companies WHERE symbol = ?`,
		symbol,
	).Scan(&r.CmpyID, &r.SecurityID, &r.Symbol, &r.Name, &etf)
	if err != nil {
		return nil, err
	}
	r.ETF = etf.Valid && etf.Int64 != 0
	return &r, nil
}

// ListPSECompanySymbols returns every symbol in the registry, ordered.
func (s *Store) ListPSECompanySymbols(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT symbol FROM pse_companies WHERE symbol IS NOT NULL ORDER BY symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// PSEEODRow is one final daily bar destined for pse_eod_prices. Volume is
// nullable: DisclosureCht.ax serves peso value but no share volume.
type PSEEODRow struct {
	Symbol      string
	TradingDate string // YYYY-MM-DD
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Value       float64
	Volume      *float64
	Source      string // "edge" | "phisix"
}

// UpsertPSEEODPrices writes final daily bars in one transaction, keyed by
// (symbol, trading_date). Caller is responsible for running rows through
// pseedge.ValidateEOD first — this helper only persists.
func (s *Store) UpsertPSEEODPrices(ctx context.Context, rows []PSEEODRow) error {
	if len(rows) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rows {
		var volume any
		if r.Volume != nil {
			volume = *r.Volume
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_eod_prices (symbol, trading_date, open, high, low, close, value, volume, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(symbol, trading_date) DO UPDATE SET
			   open = excluded.open,
			   high = excluded.high,
			   low = excluded.low,
			   close = excluded.close,
			   value = excluded.value,
			   volume = excluded.volume,
			   source = excluded.source`,
			r.Symbol, r.TradingDate, r.Open, r.High, r.Low, r.Close, r.Value, volume, r.Source,
		); err != nil {
			return fmt.Errorf("upsert pse_eod_prices %s %s: %w", r.Symbol, r.TradingDate, err)
		}
	}
	return tx.Commit()
}

// PSEIndexSnapshotRow is one per-index per-session reading destined for
// pse_index_snapshots. Breadth/summary fields are nullable — the embedded
// PSEi backfill series carries closes only, so a backfill row leaves them
// (and Change) nil.
type PSEIndexSnapshotRow struct {
	IndexCode   string
	TradingDate string // YYYY-MM-DD
	Value       float64
	Change      *float64
	PctChange   *float64
	Advances    *int64
	Declines    *int64
	Unchanged   *int64
	TotalVolume *float64
	TotalValue  *float64
	TotalTrades *int64
	Source      string
}

// UpsertPSEIndexSnapshots writes index readings in one transaction, keyed
// by (index_code, trading_date). Idempotent for the one-time series
// backfill: re-running re-asserts identical rows. Nullable columns update
// via COALESCE(excluded.col, col) so a close-only backfill row (NULL
// change/breadth/totals) can refresh value without ever clobbering the
// breadth-rich figures a post-close sync already stored for that date.
func (s *Store) UpsertPSEIndexSnapshots(ctx context.Context, rows []PSEIndexSnapshotRow) error {
	if len(rows) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_index_snapshots (index_code, trading_date, value, change, pct_change,
			   advances, declines, unchanged, total_volume, total_value, total_trades, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(index_code, trading_date) DO UPDATE SET
			   value = excluded.value,
			   change = COALESCE(excluded.change, change),
			   pct_change = COALESCE(excluded.pct_change, pct_change),
			   advances = COALESCE(excluded.advances, advances),
			   declines = COALESCE(excluded.declines, declines),
			   unchanged = COALESCE(excluded.unchanged, unchanged),
			   total_volume = COALESCE(excluded.total_volume, total_volume),
			   total_value = COALESCE(excluded.total_value, total_value),
			   total_trades = COALESCE(excluded.total_trades, total_trades),
			   source = excluded.source`,
			r.IndexCode, r.TradingDate, r.Value, nullFloat(r.Change), nullFloat(r.PctChange),
			nullInt(r.Advances), nullInt(r.Declines), nullInt(r.Unchanged),
			nullFloat(r.TotalVolume), nullFloat(r.TotalValue), nullInt(r.TotalTrades), r.Source,
		); err != nil {
			return fmt.Errorf("upsert pse_index_snapshots %s %s: %w", r.IndexCode, r.TradingDate, err)
		}
	}
	return tx.Commit()
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
