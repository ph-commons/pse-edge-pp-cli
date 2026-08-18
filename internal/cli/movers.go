// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

// moversSmallUniverse is the threshold below which the ranking is flagged
// as unrepresentative of the market.
const moversSmallUniverse = 5

// moverEntry is one ranked symbol. first_date/last_date state the actual
// sessions the pct was computed between (the symbol's stored coverage
// inside the window), so a thin series cannot masquerade as a full-window
// move. as_of is the last completed PH trading day; stale flags a series
// whose newest bar predates it.
type moverEntry struct {
	Symbol     string  `json:"symbol"`
	StartClose float64 `json:"start_close"`
	EndClose   float64 `json:"end_close"`
	Pct        float64 `json:"change_pct"`
	FirstDate  string  `json:"first_date"`
	LastDate   string  `json:"last_date"`
	Sessions   int     `json:"sessions"`
	Source     string  `json:"source"`
	AsOf       string  `json:"as_of"`
	Stale      bool    `json:"stale"`
}

type moversWindowSpan struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type moversResult struct {
	Window   moversWindowSpan `json:"window"`
	Universe int              `json:"universe"`
	Gainers  []moverEntry     `json:"gainers"`
	Losers   []moverEntry     `json:"losers"`
	Note     string           `json:"note,omitempty"`
	Source   string           `json:"source"`
	AsOf     string           `json:"as_of"`
	Stale    bool             `json:"stale"`
}

func newNovelMoversCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "movers",
		Short: "Ranked gainers and losers across the synced universe for a window, with universe size and as-of stated.",
		Long: `Use this command for ranked moves across the synced universe. Do NOT use it for one ticker's move; use 'drift' instead.

Ranks every symbol in local pse_eod_prices by percent change from its
first to its last stored close inside the window. A symbol needs at least
2 stored sessions in the window to qualify; universe is the count that
qualified — the ranking covers ONLY locally synced symbols, not the whole
exchange, and a small universe is flagged in note.`,
		Example: `  pse-edge-pp-cli movers --since 7d --json
  pse-edge-pp-cli movers --since 30d --limit 5 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("invalid --limit %d: must be positive", flagLimit))
			}

			window, err := parseSinceWindow(flagSince, 7*24*time.Hour)
			if err != nil {
				return err
			}

			asOfDay := psecal.LastCompletedTradingDay(time.Now())
			asOf := asOfDay.Format("2006-01-02")
			from := asOfDay.Add(-window).Format("2006-01-02")

			// Missing-mirror guard: typed empty OBJECT (the command's result
			// shape), never a bare [] that success-path consumers would not
			// recognize.
			var missing bool
			if dbPath, missing = missingMirrorGuard(cmd, dbPath); missing {
				return printJSONFiltered(cmd.OutOrStdout(), moversResult{
					Window:  moversWindowSpan{From: from, To: asOf},
					Gainers: []moverEntry{},
					Losers:  []moverEntry{},
					Note:    "local store has not been synced yet — run 'pse-edge-pp-cli sync market' first",
					Source:  "local",
					AsOf:    asOf,
					Stale:   true,
				}, flags)
			}

			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			hintIfUnsynced(cmd, db, "pse_eod_prices")
			hintIfStale(cmd, db, "pse_eod_prices", 72*time.Hour)

			entries, err := moversUniverse(cmd, db, from, asOf)
			if err != nil {
				return err
			}
			for i := range entries {
				entries[i].Source = "local"
				entries[i].AsOf = asOf
				entries[i].Stale = entries[i].LastDate < asOf
			}

			gainers, losers := rankMovers(entries, flagLimit)
			res := moversResult{
				Window:   moversWindowSpan{From: from, To: asOf},
				Universe: len(entries),
				Gainers:  gainers,
				Losers:   losers,
				Source:   "local",
				AsOf:     asOf,
				Stale:    moversAnyStale(entries),
			}
			if len(entries) < moversSmallUniverse {
				res.Note = fmt.Sprintf("universe is small — sync more symbols (only %d symbol(s) have >=2 local sessions in %s..%s; run 'pse-edge-pp-cli sync market' to widen it)", len(entries), from, asOf)
			}
			return printJSONFiltered(cmd.OutOrStdout(), res, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Window ending at the last completed trading day (e.g. 7d, 4w)")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Maximum entries in each of gainers and losers")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// moversUniverse computes first->last close percent change per symbol over
// [from, to], keeping symbols with at least 2 stored sessions.
func moversUniverse(cmd *cobra.Command, db *store.Store, from, to string) ([]moverEntry, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT symbol, trading_date, close FROM pse_eod_prices
		 WHERE trading_date BETWEEN ? AND ? ORDER BY symbol ASC, trading_date ASC`,
		from, to)
	if err != nil {
		if syncHintMissingTable(err) {
			return []moverEntry{}, nil
		}
		return nil, err
	}
	// Drain-first: aggregate in Go after fully consuming the cursor.
	entries := make([]moverEntry, 0)
	var cur *moverEntry
	flush := func() {
		if cur != nil && cur.Sessions >= 2 && cur.StartClose != 0 {
			cur.Pct = driftPctChange(cur.StartClose, cur.EndClose)
			entries = append(entries, *cur)
		}
		cur = nil
	}
	for rows.Next() {
		var sym, date string
		var closeP float64
		if err := rows.Scan(&sym, &date, &closeP); err != nil {
			rows.Close()
			return nil, err
		}
		if cur == nil || cur.Symbol != sym {
			flush()
			cur = &moverEntry{Symbol: sym, StartClose: closeP, FirstDate: date}
		}
		cur.EndClose = closeP
		cur.LastDate = date
		cur.Sessions++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	flush()
	return entries, nil
}

// rankMovers returns (gainers, losers): gainers are entries with pct > 0
// sorted descending, losers entries with pct < 0 sorted ascending, each
// capped at limit. Flat symbols (pct == 0) appear in neither list. Ties
// break by symbol for deterministic output.
func rankMovers(entries []moverEntry, limit int) ([]moverEntry, []moverEntry) {
	gainers := make([]moverEntry, 0, len(entries))
	losers := make([]moverEntry, 0, len(entries))
	for _, e := range entries {
		if e.Pct > 0 {
			gainers = append(gainers, e)
		} else if e.Pct < 0 {
			losers = append(losers, e)
		}
	}
	sort.Slice(gainers, func(i, j int) bool {
		if gainers[i].Pct != gainers[j].Pct {
			return gainers[i].Pct > gainers[j].Pct
		}
		return gainers[i].Symbol < gainers[j].Symbol
	})
	sort.Slice(losers, func(i, j int) bool {
		if losers[i].Pct != losers[j].Pct {
			return losers[i].Pct < losers[j].Pct
		}
		return losers[i].Symbol < losers[j].Symbol
	})
	if len(gainers) > limit {
		gainers = gainers[:limit]
	}
	if len(losers) > limit {
		losers = losers[:limit]
	}
	return gainers, losers
}

func moversAnyStale(entries []moverEntry) bool {
	for _, e := range entries {
		if e.Stale {
			return true
		}
	}
	return false
}
