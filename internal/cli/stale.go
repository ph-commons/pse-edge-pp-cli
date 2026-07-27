// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/store"
)

// staleEntry is one freshness verdict: kind "symbol" rows audit
// pse_eod_prices, kind "index" rows audit pse_index_snapshots. Every row
// carries source/as_of/stale per the red-team output contract.
type staleEntry struct {
	Kind             string   `json:"kind"` // symbol | index
	Symbol           string   `json:"symbol,omitempty"`
	IndexCode        string   `json:"index_code,omitempty"`
	LastTradingDate  string   `json:"last_trading_date"`
	FirstTradingDate string   `json:"first_trading_date"`
	Rows             int      `json:"rows"`
	LastCompleted    string   `json:"last_completed"`
	Stale            bool     `json:"stale"`
	SessionsBehind   int      `json:"sessions_behind"`
	MissingSessions  []string `json:"missing_sessions"`
	Source           string   `json:"source"`
	AsOf             string   `json:"as_of"`
}

func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var symbolsFlag []string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Per-ticker last-synced trading date versus the last completed trading day, with in-series gaps listed.",
		Long: `Use this command to audit local DB freshness and completeness. Do NOT use it to fetch data; use 'sync' instead.

For every ticker in pse_eod_prices (and every index in pse_index_snapshots)
this compares the max stored trading_date against the last completed PH
trading day per the trading calendar (16:00 Asia/Manila close gate), and
lists in-series gaps: trading days missing between the first and last stored
bar. A ticker is stale when its newest bar predates the last completed
session.`,
		Example:     "  pse-edge-pp-cli stale --json\n  pse-edge-pp-cli stale --symbols AT,GTCAP --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			// Missing-mirror guard: no database file means nothing has ever
			// been synced. Hint on stderr, empty result on stdout (stale's
			// success shape is an array, so [] stays the empty result here).
			var missing bool
			if dbPath, missing = missingMirrorGuard(cmd, dbPath); missing {
				return printJSONFiltered(cmd.OutOrStdout(), []staleEntry{}, flags)
			}

			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			hintIfUnsynced(cmd, db, "")

			now := time.Now()
			lastCompleted := psecal.LastCompletedTradingDay(now).Format("2006-01-02")
			asOf := lastCompleted

			symbolFilter := map[string]bool{}
			for _, s := range symbolsFlag {
				for _, part := range strings.Split(s, ",") {
					if part = strings.TrimSpace(strings.ToUpper(part)); part != "" {
						symbolFilter[part] = true
					}
				}
			}

			entries := make([]staleEntry, 0)

			symEntries, err := staleSymbolEntries(cmd, db, symbolFilter, lastCompleted, asOf)
			if err != nil {
				return err
			}
			entries = append(entries, symEntries...)

			// Index freshness: audited only on an unfiltered run — a
			// --symbols request is a per-ticker question.
			if len(symbolFilter) == 0 {
				idxEntries, err := staleIndexEntries(cmd, db, lastCompleted, asOf)
				if err != nil {
					return err
				}
				entries = append(entries, idxEntries...)
			}

			return printJSONFiltered(cmd.OutOrStdout(), entries, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().StringSliceVar(&symbolsFlag, "symbols", nil, "Limit the audit to these ticker symbols (comma-separated)")
	return cmd
}

// staleSymbolEntries audits pse_eod_prices per symbol. A missing table
// (store predates the market sync) yields an empty slice, not an error.
func staleSymbolEntries(cmd *cobra.Command, db *store.Store, filter map[string]bool, lastCompleted, asOf string) ([]staleEntry, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT symbol, MIN(trading_date), MAX(trading_date), COUNT(*) FROM pse_eod_prices GROUP BY symbol ORDER BY symbol`)
	if err != nil {
		if syncHintMissingTable(err) {
			return []staleEntry{}, nil
		}
		return nil, err
	}
	// Drain first: SQLite cursors must be closed before issuing the
	// per-symbol date queries below on the same pooled connection budget.
	type span struct {
		symbol, min, max string
		count            int
	}
	spans := make([]span, 0)
	for rows.Next() {
		var s span
		if err := rows.Scan(&s.symbol, &s.min, &s.max, &s.count); err != nil {
			rows.Close()
			return nil, err
		}
		spans = append(spans, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	entries := make([]staleEntry, 0, len(spans))
	for _, s := range spans {
		if len(filter) > 0 && !filter[strings.ToUpper(s.symbol)] {
			continue
		}
		missing, err := missingTradingDates(cmd, db, `SELECT trading_date FROM pse_eod_prices WHERE symbol = ?`, s.symbol, s.min, s.max)
		if err != nil {
			return nil, err
		}
		entries = append(entries, staleEntry{
			Kind:             "symbol",
			Symbol:           s.symbol,
			FirstTradingDate: s.min,
			LastTradingDate:  s.max,
			Rows:             s.count,
			LastCompleted:    lastCompleted,
			Stale:            s.max < lastCompleted,
			SessionsBehind:   len(psecal.TradingDaysBetween(s.max, lastCompleted)) + boolToInt(s.max < lastCompleted),
			MissingSessions:  missing,
			Source:           "local",
			AsOf:             asOf,
		})
	}
	return entries, nil
}

// staleIndexEntries audits pse_index_snapshots per index code.
func staleIndexEntries(cmd *cobra.Command, db *store.Store, lastCompleted, asOf string) ([]staleEntry, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT index_code, MIN(trading_date), MAX(trading_date), COUNT(*) FROM pse_index_snapshots GROUP BY index_code ORDER BY index_code`)
	if err != nil {
		if syncHintMissingTable(err) {
			return []staleEntry{}, nil
		}
		return nil, err
	}
	type span struct {
		code, min, max string
		count          int
	}
	spans := make([]span, 0)
	for rows.Next() {
		var s span
		if err := rows.Scan(&s.code, &s.min, &s.max, &s.count); err != nil {
			rows.Close()
			return nil, err
		}
		spans = append(spans, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	entries := make([]staleEntry, 0, len(spans))
	for _, s := range spans {
		missing, err := missingTradingDates(cmd, db, `SELECT trading_date FROM pse_index_snapshots WHERE index_code = ?`, s.code, s.min, s.max)
		if err != nil {
			return nil, err
		}
		entries = append(entries, staleEntry{
			Kind:             "index",
			IndexCode:        s.code,
			FirstTradingDate: s.min,
			LastTradingDate:  s.max,
			Rows:             s.count,
			LastCompleted:    lastCompleted,
			Stale:            s.max < lastCompleted,
			SessionsBehind:   len(psecal.TradingDaysBetween(s.max, lastCompleted)) + boolToInt(s.max < lastCompleted),
			MissingSessions:  missing,
			Source:           "local",
			AsOf:             asOf,
		})
	}
	return entries, nil
}

// missingTradingDates lists calendar trading days strictly between min and
// max that have no stored row for the given key.
func missingTradingDates(cmd *cobra.Command, db *store.Store, query, key, min, max string) ([]string, error) {
	rows, err := db.DB().QueryContext(cmd.Context(), query, key)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return nil, err
		}
		present[d] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	missing := make([]string, 0)
	for _, d := range psecal.TradingDaysBetween(min, max) {
		if !present[d] {
			missing = append(missing, d)
		}
	}
	return missing, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
