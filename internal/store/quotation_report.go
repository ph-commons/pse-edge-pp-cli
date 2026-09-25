// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Daily Quotation Report rows. Created lazily, like EnsurePSEEdgeTables.
// These statements stay out of pseEdgeMigrations and StoreSchemaVersion.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrQuotationRevisionNotFound means the requested SHA is absent or incomplete.
var ErrQuotationRevisionNotFound = errors.New("quotation revision not found")

var quotationReportMigrations = []string{
	`CREATE TABLE IF NOT EXISTS pse_quotation_revisions (
		session_date TEXT NOT NULL,
		source_sha256 TEXT NOT NULL,
		source_url TEXT NOT NULL,
		byte_length INTEGER NOT NULL,
		acquired_at TEXT NOT NULL,
		pdf_path TEXT NOT NULL,
		listing_url TEXT NOT NULL,
		ajax_url TEXT NOT NULL,
		matched_title TEXT NOT NULL,
		matched_category TEXT NOT NULL,
		upload_date TEXT NOT NULL,
		document_session_date TEXT NOT NULL,
		is_current INTEGER NOT NULL,
		complete INTEGER NOT NULL,
		PRIMARY KEY (session_date, source_sha256)
	)`,
	`CREATE TABLE IF NOT EXISTS pse_quotation_rows (
		session_date TEXT NOT NULL,
		source_sha256 TEXT NOT NULL,
		symbol TEXT NOT NULL,
		issue_name TEXT NOT NULL,
		board TEXT NOT NULL,
		security_class TEXT NOT NULL,
		currency TEXT NOT NULL,
		row_locator TEXT NOT NULL,
		page INTEGER NOT NULL,
		bid REAL NULL,
		ask REAL NULL,
		open REAL NULL,
		high REAL NULL,
		low REAL NULL,
		close REAL NULL,
		volume REAL NULL,
		value REAL NULL,
		net_foreign REAL NULL,
		field_status TEXT NOT NULL,
		PRIMARY KEY (session_date, source_sha256, row_locator)
	)`,
	`CREATE TABLE IF NOT EXISTS pse_quotation_ambiguous (
		session_date TEXT NOT NULL,
		source_sha256 TEXT NOT NULL,
		symbol TEXT NOT NULL,
		locators TEXT NOT NULL,
		PRIMARY KEY (session_date, source_sha256, symbol)
	)`,
}

// QuotationStoredRow is one security line. Nil measures stay NULL.
type QuotationStoredRow struct {
	Symbol        string
	IssueName     string
	Board         string
	SecurityClass string
	Currency      string
	RowLocator    string
	Page          int
	Bid           *float64
	Ask           *float64
	Open          *float64
	High          *float64
	Low           *float64
	Close         *float64
	Volume        *float64
	Value         *float64
	NetForeign    *float64
	FieldStatus   map[string]string
}

// QuotationAmbiguousRow is a symbol that was not admitted as a security row.
type QuotationAmbiguousRow struct {
	Symbol   string
	Locators []string
}

// QuotationRevisionInput is one complete admitted revision.
type QuotationRevisionInput struct {
	SessionDate         string
	SourceSHA256        string
	SourceURL           string
	ByteLength          int
	AcquiredAt          string
	PDFPath             string
	ListingURL          string
	AjaxURL             string
	MatchedTitle        string
	MatchedCategory     string
	UploadDate          string
	DocumentSessionDate string
	Rows                []QuotationStoredRow
	Ambiguous           []QuotationAmbiguousRow
}

// QuotationRevisionView is one stored revision plus its rows.
type QuotationRevisionView struct {
	QuotationRevisionInput
	Current bool
}

// QuotationQuery selects admitted revisions. An empty SHA returns current rows.
type QuotationQuery struct {
	From string
	To   string
	SHA  string
}

// EnsureQuotationReportTables creates the three quotation tables.
// It does not create pse_eod_prices and EnsurePSEEdgeTables does not create these.
func (s *Store) EnsureQuotationReportTables(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.createQuotationReportTables(ctx)
}

func (s *Store) createQuotationReportTables(ctx context.Context) error {
	for _, stmt := range quotationReportMigrations {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("creating quotation report table: %w", err)
		}
	}
	return nil
}

// PersistQuotationRevision replaces one SHA and marks it current, in one transaction.
func (s *Store) PersistQuotationRevision(ctx context.Context, in QuotationRevisionInput) error {
	return s.persistQuotationRevision(ctx, in, true, nil)
}

// StoreQuotationRevision replaces one SHA and leaves the current pointer unchanged.
func (s *Store) StoreQuotationRevision(ctx context.Context, in QuotationRevisionInput) error {
	return s.persistQuotationRevision(ctx, in, false, nil)
}

// PromoteQuotationRevision marks one complete SHA current and clears the others for that session.
func (s *Store) PromoteQuotationRevision(ctx context.Context, session, sha string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.createQuotationReportTables(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var complete int
	err = tx.QueryRowContext(ctx,
		`SELECT complete FROM pse_quotation_revisions WHERE session_date = ? AND source_sha256 = ?`,
		session, sha,
	).Scan(&complete)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrQuotationRevisionNotFound, sha)
		}
		return err
	}
	if complete != 1 {
		return fmt.Errorf("%w: %s", ErrQuotationRevisionNotFound, sha)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE pse_quotation_revisions SET is_current = 0 WHERE session_date = ?`, session,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE pse_quotation_revisions SET is_current = 1 WHERE session_date = ? AND source_sha256 = ?`,
		session, sha,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearQuotationCurrent clears is_current for one session. Rows stay complete.
func (s *Store) ClearQuotationCurrent(ctx context.Context, session string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.createQuotationReportTables(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE pse_quotation_revisions SET is_current = 0 WHERE session_date = ?`, session)
	return err
}

// persistQuotationRevision writes the revision. makeCurrent also demotes the other SHAs.
// beforeCommit runs before commit. A non-nil error from beforeCommit rolls the transaction back.
func (s *Store) persistQuotationRevision(ctx context.Context, in QuotationRevisionInput, makeCurrent bool, beforeCommit func() error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.createQuotationReportTables(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pse_quotation_rows WHERE session_date = ? AND source_sha256 = ?`,
		in.SessionDate, in.SourceSHA256); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pse_quotation_ambiguous WHERE session_date = ? AND source_sha256 = ?`,
		in.SessionDate, in.SourceSHA256); err != nil {
		return err
	}
	for i, row := range in.Rows {
		status, err := statusJSON(row.FieldStatus)
		if err != nil {
			return err
		}
		// A missing locator is not unique. Keep the JSON evidence unchanged
		// and store a stable stand-in so two blank locators can both be admitted.
		locator := row.RowLocator
		if locator == "" {
			locator = fmt.Sprintf("~%d", i+1)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_quotation_rows (
				session_date, source_sha256, symbol, issue_name, board, security_class, currency, row_locator, page,
				bid, ask, open, high, low, close, volume, value, net_foreign, field_status
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.SessionDate, in.SourceSHA256, row.Symbol, row.IssueName, row.Board, row.SecurityClass, row.Currency, locator, row.Page,
			bindFloat(row.Bid), bindFloat(row.Ask), bindFloat(row.Open), bindFloat(row.High), bindFloat(row.Low), bindFloat(row.Close), bindFloat(row.Volume), bindFloat(row.Value), bindFloat(row.NetForeign),
			status,
		); err != nil {
			return fmt.Errorf("insert quotation row %s: %w", row.RowLocator, err)
		}
	}
	for _, item := range in.Ambiguous {
		locators, err := locatorsJSON(item.Locators)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pse_quotation_ambiguous (session_date, source_sha256, symbol, locators) VALUES (?, ?, ?, ?)`,
			in.SessionDate, in.SourceSHA256, item.Symbol, locators,
		); err != nil {
			return fmt.Errorf("insert quotation ambiguous %s: %w", item.Symbol, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO pse_quotation_revisions (
			session_date, source_sha256, source_url, byte_length, acquired_at, pdf_path,
			listing_url, ajax_url, matched_title, matched_category, upload_date, document_session_date,
			is_current, complete
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(session_date, source_sha256) DO UPDATE SET
			source_url = excluded.source_url,
			byte_length = excluded.byte_length,
			acquired_at = excluded.acquired_at,
			pdf_path = excluded.pdf_path,
			listing_url = excluded.listing_url,
			ajax_url = excluded.ajax_url,
			matched_title = excluded.matched_title,
			matched_category = excluded.matched_category,
			upload_date = excluded.upload_date,
			document_session_date = excluded.document_session_date,
			complete = 1,
			is_current = CASE WHEN excluded.is_current = 1 THEN 1 ELSE pse_quotation_revisions.is_current END`,
		in.SessionDate, in.SourceSHA256, in.SourceURL, in.ByteLength, in.AcquiredAt, in.PDFPath,
		in.ListingURL, in.AjaxURL, in.MatchedTitle, in.MatchedCategory, in.UploadDate, in.DocumentSessionDate,
		currentBit(makeCurrent),
	); err != nil {
		return err
	}
	if makeCurrent {
		if _, err := tx.ExecContext(ctx,
			`UPDATE pse_quotation_revisions SET is_current = 0 WHERE session_date = ? AND source_sha256 <> ?`,
			in.SessionDate, in.SourceSHA256); err != nil {
			return err
		}
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func currentBit(makeCurrent bool) int {
	if makeCurrent {
		return 1
	}
	return 0
}

// QueryQuotationRevisions returns complete revisions. SHA empty means is_current = 1.
// A SHA that is missing or not complete returns ErrQuotationRevisionNotFound.
func (s *Store) QueryQuotationRevisions(ctx context.Context, q QuotationQuery) ([]QuotationRevisionView, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'pse_quotation_revisions'`,
	).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		if q.SHA != "" {
			return nil, fmt.Errorf("%w: %s", ErrQuotationRevisionNotFound, q.SHA)
		}
		return []QuotationRevisionView{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_date, source_sha256, source_url, byte_length, acquired_at, pdf_path,
			listing_url, ajax_url, matched_title, matched_category, upload_date, document_session_date, is_current
		FROM pse_quotation_revisions
		WHERE complete = 1
			AND session_date >= ?
			AND session_date <= ?
			AND (
				(? = '' AND is_current = 1)
				OR (? <> '' AND source_sha256 = ?)
			)
		ORDER BY session_date, source_sha256`,
		q.From, q.To, q.SHA, q.SHA, q.SHA,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var headers []QuotationRevisionView
	for rows.Next() {
		var view QuotationRevisionView
		var current int
		if err := rows.Scan(
			&view.SessionDate, &view.SourceSHA256, &view.SourceURL, &view.ByteLength, &view.AcquiredAt, &view.PDFPath,
			&view.ListingURL, &view.AjaxURL, &view.MatchedTitle, &view.MatchedCategory, &view.UploadDate, &view.DocumentSessionDate, &current,
		); err != nil {
			return nil, err
		}
		view.Current = current == 1
		headers = append(headers, view)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]QuotationRevisionView, 0, len(headers))
	for _, view := range headers {
		loaded, err := s.loadQuotationBody(ctx, view)
		if err != nil {
			return nil, err
		}
		out = append(out, loaded)
	}
	if q.SHA != "" && len(out) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrQuotationRevisionNotFound, q.SHA)
	}
	return out, nil
}

func (s *Store) loadQuotationBody(ctx context.Context, view QuotationRevisionView) (QuotationRevisionView, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol, issue_name, board, security_class, currency, row_locator, page,
			bid, ask, open, high, low, close, volume, value, net_foreign, field_status
		FROM pse_quotation_rows
		WHERE session_date = ? AND source_sha256 = ?
		ORDER BY page, row_locator`,
		view.SessionDate, view.SourceSHA256,
	)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	view.Rows = []QuotationStoredRow{}
	for rows.Next() {
		var row QuotationStoredRow
		var bid, ask, open, high, low, close, volume, value, net sql.NullFloat64
		var status string
		if err := rows.Scan(
			&row.Symbol, &row.IssueName, &row.Board, &row.SecurityClass, &row.Currency, &row.RowLocator, &row.Page,
			&bid, &ask, &open, &high, &low, &close, &volume, &value, &net, &status,
		); err != nil {
			return view, err
		}
		row.Bid = scanFloat(bid)
		row.Ask = scanFloat(ask)
		row.Open = scanFloat(open)
		row.High = scanFloat(high)
		row.Low = scanFloat(low)
		row.Close = scanFloat(close)
		row.Volume = scanFloat(volume)
		row.Value = scanFloat(value)
		row.NetForeign = scanFloat(net)
		row.FieldStatus = map[string]string{}
		if err := json.Unmarshal([]byte(status), &row.FieldStatus); err != nil {
			return view, fmt.Errorf("field_status %s: %w", row.RowLocator, err)
		}
		view.Rows = append(view.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return view, err
	}
	if err := rows.Close(); err != nil {
		return view, err
	}
	amb, err := s.db.QueryContext(ctx,
		`SELECT symbol, locators FROM pse_quotation_ambiguous
		WHERE session_date = ? AND source_sha256 = ?
		ORDER BY symbol`,
		view.SessionDate, view.SourceSHA256,
	)
	if err != nil {
		return view, err
	}
	defer amb.Close()
	view.Ambiguous = []QuotationAmbiguousRow{}
	for amb.Next() {
		var item QuotationAmbiguousRow
		var locators string
		if err := amb.Scan(&item.Symbol, &locators); err != nil {
			return view, err
		}
		item.Locators = []string{}
		if err := json.Unmarshal([]byte(locators), &item.Locators); err != nil {
			return view, fmt.Errorf("locators %s: %w", item.Symbol, err)
		}
		if item.Locators == nil {
			item.Locators = []string{}
		}
		view.Ambiguous = append(view.Ambiguous, item)
	}
	return view, amb.Err()
}

func bindFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func scanFloat(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func statusJSON(m map[string]string) (string, error) {
	if m == nil {
		m = map[string]string{}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func locatorsJSON(s []string) (string, error) {
	if s == nil {
		s = []string{}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
