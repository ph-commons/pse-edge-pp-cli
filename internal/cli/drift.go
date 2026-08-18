// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

// driftMinBandCoverDays is the minimum count of locally stored trading
// sessions inside the trailing 52-week span required before the 52w band
// position is reported. Below it the "52-week" high/low are only a
// partial-window observation, so band_position_pct stays null and the
// note states the actual coverage.
const driftMinBandCoverDays = 200

// driftBandSpan is the trailing calendar span behind the "52-week"
// high/low band.
const driftBandSpan = 364 * 24 * time.Hour

type driftWindow struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Sessions int    `json:"sessions"`
}

// driftResult is the typed drift verdict. Pointer fields are null (not
// zero) when the local data cannot support the figure; Note says why.
type driftResult struct {
	Symbol          string      `json:"symbol"`
	Window          driftWindow `json:"window"`
	StartClose      *float64    `json:"start_close"`
	EndClose        *float64    `json:"end_close"`
	ChangePct       *float64    `json:"change_pct"`
	PseiChangePct   *float64    `json:"psei_change_pct"`
	RelativePct     *float64    `json:"relative_pct"`
	High52w         *float64    `json:"high_52w"`
	Low52w          *float64    `json:"low_52w"`
	BandPositionPct *float64    `json:"band_position_pct"`
	CoverDays       int         `json:"cover_days"`
	Note            string      `json:"note,omitempty"`
	Source          string      `json:"source"`
	AsOf            string      `json:"as_of"`
	Stale           bool        `json:"stale"`
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "drift <SYMBOL>",
		Short: "Percent change over a window, absolute and versus the PSEi over the same sessions, with 52-week band position.",
		Long: `Use this command for performance over a window and vs-PSEi comparison. Do NOT use it for raw daily rows ('history') or today's price ('quote').

Computed entirely from the local store: change_pct is first-close to
last-close over the stored sessions inside the window; psei_change_pct is
the PSEi move between those same first/last session dates (aligned, from
pse_index_snapshots); relative_pct = change_pct - psei_change_pct.

band_position_pct places the last close inside the trailing 52-week
high/low band (0 = at the low, 100 = at the high). It is null when fewer
than 200 trading sessions are stored locally in that span — cover_days
states the actual coverage, so partial-window extremes are never passed
off as true 52-week figures. Fewer than 2 stored sessions in the window
yields a typed empty result with a note, exit 0.`,
		Example: `  pse-edge-pp-cli drift AT --since 90d --json
  pse-edge-pp-cli drift GTCAP --since 4w --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("drift takes exactly one symbol, got %d arguments", len(args)))
			}
			sym, err := validatePSESymbol(args[0])
			if err != nil {
				return err
			}

			window, err := parseSinceWindow(flagSince, 90*24*time.Hour)
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
				return printJSONFiltered(cmd.OutOrStdout(), driftResult{
					Symbol: sym,
					Window: driftWindow{From: from, To: asOf},
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

			hintIfUnsynced(cmd, db, "pse_eod_prices")
			hintIfStale(cmd, db, "pse_eod_prices", 72*time.Hour)

			res := driftResult{
				Symbol: sym,
				Window: driftWindow{From: from, To: asOf},
				Source: "local",
				AsOf:   asOf,
			}

			// Window bars, ascending. Drain-first before further queries.
			bars, err := driftWindowBars(cmd, db, sym, from, asOf)
			if err != nil {
				return err
			}
			res.Window.Sessions = len(bars)

			res.Stale, err = analyticsSeriesStale(cmd, db, `SELECT MAX(trading_date) FROM pse_eod_prices WHERE symbol = ?`, sym, asOf)
			if err != nil {
				return err
			}

			if len(bars) < 2 {
				res.Note = fmt.Sprintf("insufficient local data: %d stored session(s) for %s in %s..%s (need >=2). Run 'pse-edge-pp-cli sync market --symbols %s' first.",
					len(bars), sym, from, asOf, sym)
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}

			first, last := bars[0], bars[len(bars)-1]
			startClose, endClose := first.close, last.close
			res.StartClose = &startClose
			res.EndClose = &endClose
			change := driftPctChange(startClose, endClose)
			res.ChangePct = &change

			// PSEi over the SAME first/last session dates.
			pseiChange, pseiNote, err := driftPseiChange(cmd, db, first.date, last.date)
			if err != nil {
				return err
			}
			res.PseiChangePct = pseiChange
			if pseiChange != nil {
				rel := change - *pseiChange
				res.RelativePct = &rel
			}

			// Trailing 52-week band ending at the window end.
			span52From := asOfDay.Add(-driftBandSpan).Format("2006-01-02")
			high52, low52, coverDays, err := drift52wSpan(cmd, db, sym, span52From, asOf)
			if err != nil {
				return err
			}
			res.CoverDays = coverDays
			res.High52w = high52
			res.Low52w = low52
			var bandNote string
			if high52 != nil && low52 != nil {
				res.BandPositionPct, bandNote = driftBandPosition(endClose, *low52, *high52, coverDays, driftMinBandCoverDays)
			}
			res.Note = strings.TrimSpace(strings.Join([]string{pseiNote, bandNote}, " "))

			return printJSONFiltered(cmd.OutOrStdout(), res, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "90d", "Performance window ending at the last completed trading day (e.g. 90d, 12w)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

type driftBar struct {
	date  string
	close float64
}

// driftWindowBars reads (date, close) bars for sym in [from, to] ascending.
func driftWindowBars(cmd *cobra.Command, db *store.Store, sym, from, to string) ([]driftBar, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT trading_date, close FROM pse_eod_prices
		 WHERE symbol = ? AND trading_date BETWEEN ? AND ? ORDER BY trading_date ASC`,
		sym, from, to)
	if err != nil {
		if syncHintMissingTable(err) {
			return []driftBar{}, nil
		}
		return nil, err
	}
	out := make([]driftBar, 0)
	for rows.Next() {
		var b driftBar
		if err := rows.Scan(&b.date, &b.close); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// driftPseiChange computes the PSEi percent change between two exact
// session dates from pse_index_snapshots. Either date missing locally
// yields (nil, note) — never a nearest-date substitution.
func driftPseiChange(cmd *cobra.Command, db *store.Store, firstDate, lastDate string) (*float64, string, error) {
	value := func(date string) (*float64, error) {
		var v float64
		err := db.DB().QueryRowContext(cmd.Context(),
			`SELECT value FROM pse_index_snapshots WHERE index_code = 'PSEI' AND trading_date = ?`, date).Scan(&v)
		if err != nil {
			if err == sql.ErrNoRows || syncHintMissingTable(err) {
				return nil, nil
			}
			return nil, err
		}
		return &v, nil
	}
	start, err := value(firstDate)
	if err != nil {
		return nil, "", err
	}
	end, err := value(lastDate)
	if err != nil {
		return nil, "", err
	}
	if start == nil || end == nil {
		return nil, fmt.Sprintf("psei_change_pct unavailable: PSEI snapshot missing locally for %s and/or %s.", firstDate, lastDate), nil
	}
	if *start == 0 {
		return nil, "psei_change_pct unavailable: stored PSEI value is 0 on the aligned start session.", nil
	}
	change := driftPctChange(*start, *end)
	return &change, "", nil
}

// drift52wSpan returns max(high), min(low), and the stored-session count
// for sym over [from, to]. All nil/0 when nothing is stored in the span.
func drift52wSpan(cmd *cobra.Command, db *store.Store, sym, from, to string) (*float64, *float64, int, error) {
	var high, low sql.NullFloat64
	var count int
	err := db.DB().QueryRowContext(cmd.Context(),
		`SELECT MAX(high), MIN(low), COUNT(*) FROM pse_eod_prices
		 WHERE symbol = ? AND trading_date BETWEEN ? AND ?`,
		sym, from, to).Scan(&high, &low, &count)
	if err != nil {
		if syncHintMissingTable(err) {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, err
	}
	var h, l *float64
	if high.Valid {
		v := high.Float64
		h = &v
	}
	if low.Valid {
		v := low.Float64
		l = &v
	}
	return h, l, count, nil
}

// driftPctChange is the percent change from start to end.
func driftPctChange(start, end float64) float64 {
	return (end - start) / start * 100
}

// driftBandPosition places endClose within the [low, high] band as a
// 0-100 percentage. Returns (nil, note) when coverage is below minCover
// sessions (the band is not a true 52-week band) or when the band is
// degenerate (high == low).
func driftBandPosition(endClose, low, high float64, coverDays, minCover int) (*float64, string) {
	if coverDays < minCover {
		return nil, fmt.Sprintf("band_position_pct null: only %d trading session(s) stored locally in the trailing 52 weeks (need >=%d for an honest 52-week band).", coverDays, minCover)
	}
	if high == low {
		return nil, "band_position_pct null: degenerate 52-week band (high == low)."
	}
	pos := (endClose - low) / (high - low) * 100
	return &pos, ""
}
