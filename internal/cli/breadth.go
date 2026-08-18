// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

// breadthRow is one PSEI session with breadth integers present.
// adv_dec_ratio is null when declines is 0 (never a divide-by-zero or a
// fabricated large number).
type breadthRow struct {
	Date        string   `json:"date"`
	Advances    int64    `json:"advances"`
	Declines    int64    `json:"declines"`
	Unchanged   *int64   `json:"unchanged"`
	AdvDecRatio *float64 `json:"adv_dec_ratio"`
	TotalValue  *float64 `json:"total_value"`
	Source      string   `json:"source"`
	AsOf        string   `json:"as_of"`
	Stale       bool     `json:"stale"`
}

type breadthSummary struct {
	Days          int      `json:"days"`
	AvgRatio      *float64 `json:"avg_ratio"`
	AdvancingDays int      `json:"advancing_days"`
	DecliningDays int      `json:"declining_days"`
}

type breadthCoverage struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Days  int    `json:"days"`
}

type breadthResult struct {
	Rows            []breadthRow     `json:"rows"`
	Summary         breadthSummary   `json:"summary"`
	BreadthCoverage *breadthCoverage `json:"breadth_coverage"`
	Note            string           `json:"note,omitempty"`
	Source          string           `json:"source"`
	AsOf            string           `json:"as_of"`
	Stale           bool             `json:"stale"`
}

func newNovelBreadthCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "breadth",
		Short: "Advance/decline/unchanged and value-traded trend over time from local index snapshots.",
		Long: `Use this command for breadth over time from local snapshots. Do NOT use it for today's snapshot; use 'market' instead.

Reads PSEI rows from pse_index_snapshots that actually carry breadth
integers (advances IS NOT NULL). The 2021-2025 embedded backfill series is
close-only and carries no breadth, so those dates are excluded rather than
zero-filled; breadth_coverage states the first/last/count of sessions that
do have breadth locally. Breadth accumulates one session per post-close
'sync market' run.`,
		Example: `  pse-edge-pp-cli breadth --since 30d --json
  pse-edge-pp-cli breadth --since 12w --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			window, err := parseSinceWindow(flagSince, 30*24*time.Hour)
			if err != nil {
				return err
			}

			asOfDay := psecal.LastCompletedTradingDay(time.Now())
			asOf := asOfDay.Format("2006-01-02")
			from := asOfDay.Add(-window).Format("2006-01-02")

			// Missing-mirror guard: typed empty OBJECT (the command's result
			// shape), never a bare [].
			var missing bool
			if dbPath, missing = missingMirrorGuard(cmd, dbPath); missing {
				return printJSONFiltered(cmd.OutOrStdout(), breadthResult{
					Rows:   []breadthRow{},
					Note:   "local store has not been synced yet — run 'pse-edge-pp-cli sync market' first",
					Source: "local",
					AsOf:   asOf,
					Stale:  true,
				}, flags)
			}

			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			hintIfUnsynced(cmd, db, "pse_index_snapshots")
			hintIfStale(cmd, db, "pse_index_snapshots", 72*time.Hour)

			rows, err := breadthWindowRows(cmd, db, from, asOf)
			if err != nil {
				return err
			}

			// Overall breadth coverage (all-time, not just the window).
			coverage, err := breadthCoverageSpan(cmd, db)
			if err != nil {
				return err
			}

			stale := true
			if coverage != nil {
				stale = coverage.Last < asOf
			}
			for i := range rows {
				rows[i].Source = "local"
				rows[i].AsOf = asOf
				rows[i].Stale = stale
			}

			res := breadthResult{
				Rows:            rows,
				Summary:         breadthSummarize(rows),
				BreadthCoverage: coverage,
				Note:            "rows without breadth integers (e.g. the 2021-2025 close-only backfill series) are excluded, not zero-filled; breadth_coverage lists the sessions that have breadth locally.",
				Source:          "local",
				AsOf:            asOf,
				Stale:           stale,
			}
			return printJSONFiltered(cmd.OutOrStdout(), res, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Window ending at the last completed trading day (e.g. 30d, 12w)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// breadthWindowRows reads PSEI sessions with breadth in [from, to]
// ascending. BOTH advances and declines must be non-NULL: a row with only
// one of them cannot support a ratio or an advancing/declining verdict,
// and rendering the NULL side as 0 would fabricate a figure — such rows
// are excluded like the close-only backfill series.
func breadthWindowRows(cmd *cobra.Command, db *store.Store, from, to string) ([]breadthRow, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT trading_date, advances, declines, unchanged, total_value FROM pse_index_snapshots
		 WHERE index_code = 'PSEI' AND advances IS NOT NULL AND declines IS NOT NULL AND trading_date BETWEEN ? AND ?
		 ORDER BY trading_date ASC`,
		from, to)
	if err != nil {
		if syncHintMissingTable(err) {
			return []breadthRow{}, nil
		}
		return nil, err
	}
	// Drain-first before any further queries.
	out := make([]breadthRow, 0)
	for rows.Next() {
		var date string
		var advances int64
		var declines, unchanged sql.NullInt64
		var totalValue sql.NullFloat64
		if err := rows.Scan(&date, &advances, &declines, &unchanged, &totalValue); err != nil {
			rows.Close()
			return nil, err
		}
		r := breadthRow{Date: date, Advances: advances}
		if declines.Valid {
			r.Declines = declines.Int64
		}
		if unchanged.Valid {
			v := unchanged.Int64
			r.Unchanged = &v
		}
		if totalValue.Valid {
			v := totalValue.Float64
			r.TotalValue = &v
		}
		r.AdvDecRatio = breadthAdvDecRatio(r.Advances, r.Declines)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// breadthCoverageSpan reports first/last/count over ALL locally stored
// breadth-bearing PSEI sessions. Nil when none exist.
func breadthCoverageSpan(cmd *cobra.Command, db *store.Store) (*breadthCoverage, error) {
	var first, last sql.NullString
	var days int
	err := db.DB().QueryRowContext(cmd.Context(),
		`SELECT MIN(trading_date), MAX(trading_date), COUNT(*) FROM pse_index_snapshots
		 WHERE index_code = 'PSEI' AND advances IS NOT NULL AND declines IS NOT NULL`).Scan(&first, &last, &days)
	if err != nil {
		if syncHintMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	if !first.Valid || days == 0 {
		return nil, nil
	}
	return &breadthCoverage{First: first.String, Last: last.String, Days: days}, nil
}

// breadthAdvDecRatio is advances/declines; nil when declines is 0.
func breadthAdvDecRatio(advances, declines int64) *float64 {
	if declines == 0 {
		return nil
	}
	r := float64(advances) / float64(declines)
	return &r
}

// breadthSummarize aggregates window rows: mean ratio over sessions with a
// defined ratio, and counts of net-advancing/net-declining sessions.
func breadthSummarize(rows []breadthRow) breadthSummary {
	s := breadthSummary{Days: len(rows)}
	ratioSum := 0.0
	ratioN := 0
	for _, r := range rows {
		if r.AdvDecRatio != nil {
			ratioSum += *r.AdvDecRatio
			ratioN++
		}
		switch {
		case r.Advances > r.Declines:
			s.AdvancingDays++
		case r.Declines > r.Advances:
			s.DecliningDays++
		}
	}
	if ratioN > 0 {
		avg := ratioSum / float64(ratioN)
		s.AvgRatio = &avg
	}
	return s
}
