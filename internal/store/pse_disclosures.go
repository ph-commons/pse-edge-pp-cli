// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored pse_disclosures writers/readers. Sibling of
// pse_edge_migrations.go (which owns the CREATE TABLE); kept in its own
// file so concurrent hand-authored work never edits the shared migration
// file.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PSEDisclosureRow is one disclosure header destined for pse_disclosures.
type PSEDisclosureRow struct {
	EdgeNo      string
	CmpyID      int
	Symbol      string
	Template    string
	Title       string
	DisclosedAt string // RFC3339
}

// UpsertPSEDisclosures inserts or refreshes disclosure headers in one
// transaction, keyed by edge_no. synced_at is stamped UTC RFC3339 at write
// time. An empty Symbol never overwrites a previously-resolved one.
func (s *Store) UpsertPSEDisclosures(ctx context.Context, rows []PSEDisclosureRow) error {
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
		if r.EdgeNo == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_disclosures (edge_no, cmpy_id, symbol, template, title, disclosed_at, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(edge_no) DO UPDATE SET
			   cmpy_id = excluded.cmpy_id,
			   symbol = CASE WHEN excluded.symbol = '' THEN pse_disclosures.symbol ELSE excluded.symbol END,
			   template = excluded.template,
			   title = excluded.title,
			   disclosed_at = excluded.disclosed_at,
			   synced_at = excluded.synced_at`,
			r.EdgeNo, r.CmpyID, r.Symbol, r.Template, r.Title, r.DisclosedAt, now,
		); err != nil {
			return fmt.Errorf("upsert pse_disclosures %s: %w", r.EdgeNo, err)
		}
	}
	return tx.Commit()
}

// LookupPSECompanyByCmpyID resolves a cmpy_id to its registry row. Returns
// sql.ErrNoRows (wrapped) when the ID is not in the local registry.
func (s *Store) LookupPSECompanyByCmpyID(ctx context.Context, cmpyID int) (*PSECompanyRow, error) {
	var r PSECompanyRow
	var etf sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT cmpy_id, security_id, symbol, name, COALESCE(etf, 0) FROM pse_companies WHERE cmpy_id = ?`,
		cmpyID,
	).Scan(&r.CmpyID, &r.SecurityID, &r.Symbol, &r.Name, &etf)
	if err != nil {
		return nil, err
	}
	r.ETF = etf.Valid && etf.Int64 != 0
	return &r, nil
}
