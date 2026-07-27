// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored porcelain: dual-source EOD quote (PSE Edge stockData.do +
// phisix fast path) with as-of/stale discipline and cross-source
// divergence detection. Registered via the registerNovelCommand hook.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ngpestelos/pse-edge-pp-cli/internal/client"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/pseedge"
	"github.com/spf13/cobra"
)

// phisixStockURL is the community phisix mirror (unknown-operator redeploy;
// optional fast path only — edge is first-party and wins on conflict).
const phisixStockURL = "https://phisix-api3.appspot.com/stocks/%s.json"

// quoteRow is one per-symbol quote. Nullable numerics are pointers so a
// closed session serves explicit nulls, never zeros.
type quoteRow struct {
	Symbol     string   `json:"symbol"`
	Name       string   `json:"name"`
	Close      *float64 `json:"close"`
	Change     *float64 `json:"change"`
	ChangePct  *float64 `json:"change_pct"`
	Open       *float64 `json:"open"`
	High       *float64 `json:"high"`
	Low        *float64 `json:"low"`
	PrevClose  *float64 `json:"prev_close"`
	Volume     *float64 `json:"volume"`
	Value      *float64 `json:"value"`
	MarketCap  *float64 `json:"market_cap"`
	AsOf       string   `json:"as_of"`
	Stale      bool     `json:"stale"`
	Source     string   `json:"source"`
	Divergence bool     `json:"divergence"`
	Note       string   `json:"note,omitempty"`
}

// phisixResponse is the phisix stocks/{SYM}.json shape. as_of is a
// synthetic midnight stamp and deliberately ignored for dating.
type phisixResponse struct {
	Stocks []struct {
		Name  string `json:"name"`
		Price struct {
			Amount float64 `json:"amount"`
		} `json:"price"`
		PercentChange float64 `json:"percentChange"`
		Volume        float64 `json:"volume"`
		Symbol        string  `json:"symbol"`
	} `json:"stocks"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newQuoteCmd(flags))
	})
}

func newQuoteCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "quote <symbol> [symbol...]",
		Short: "Dual-source EOD quote per ticker: close/change/volume with as-of date, staleness, and edge-vs-phisix divergence flag",
		Long: `Use this command for the latest end-of-day quote of one or more tickers.
Do NOT use it for ranged history ('history') or window performance ('drift').

Two sources are consulted per symbol and merged:

  edge    companyPage/stockData.do — first-party; preferred for OHLC,
          value traded, and market cap (and for close on conflict)
  phisix  phisix-api3.appspot.com — community fast path for
          close/percent-change/volume

Either source may fail alone (the row's source field says which answered);
both failing is a hard typed error. When both serve a close and they differ
by more than 0.01, divergence:true is set, a warning goes to stderr, and
the edge close wins. as_of is the last completed PH trading session per the
trading calendar; before the 16:00 Asia/Manila close gate rows are marked
stale. A blank edge change cell on a non-trading day is served as explicit
nulls with a "closed-session" note, never zeros.`,
		Example: `  pse-edge-pp-cli quote AT --json
  pse-edge-pp-cli quote AT GTCAP HTI --json --select symbol,close`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true
			if dbPath == "" {
				dbPath = defaultDBPath("pse-edge-pp-cli")
			}

			state := psecal.SessionState(time.Now())
			// Dedup while preserving caller order.
			syms := make([]string, 0, len(args))
			seen := map[string]bool{}
			for _, arg := range args {
				sym := strings.ToUpper(strings.TrimSpace(arg))
				if sym == "" || seen[sym] {
					continue
				}
				seen[sym] = true
				syms = append(syms, sym)
			}
			// Parallelize multi-ticker quotes (each symbol still dual-sources
			// edge+phisix concurrently inside fetchQuote). Cap workers so a
			// long list cannot open unbounded connections to EDGE.
			const maxQuoteWorkers = 6
			type result struct {
				row *quoteRow
				err error
			}
			results := make([]result, len(syms))
			sem := make(chan struct{}, maxQuoteWorkers)
			var wg sync.WaitGroup
			for i, sym := range syms {
				wg.Add(1)
				go func(i int, sym string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					row, err := fetchQuote(cmd, flags, c, dbPath, sym, state)
					results[i] = result{row: row, err: err}
				}(i, sym)
			}
			wg.Wait()
			rows := make([]quoteRow, 0, len(syms))
			for _, r := range results {
				if r.err != nil {
					return r.err
				}
				rows = append(rows, *r.row)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path for symbol resolution (default: resolved data directory data.db)")
	return cmd
}

// fetchQuote resolves one symbol and merges the edge and phisix answers.
func fetchQuote(cmd *cobra.Command, flags *rootFlags, c *client.Client, dbPath, sym string, state psecal.State) (*quoteRow, error) {
	rc, err := resolvePSECompany(cmd.Context(), cmd, flags, dbPath, sym)
	if err != nil {
		return nil, err
	}

	row := &quoteRow{
		Symbol: rc.Symbol,
		Name:   rc.Name,
		AsOf:   state.LastCompleted,
		// Provisional before the close gate on a trading day.
		Stale: liveFetchStale(state),
	}

	// Edge + phisix in parallel (independent sources; client.Get is concurrent-safe
	// for response bodies; lastContentType is best-effort and may race).
	var (
		snap      *pseedge.Snapshot
		edgeErr   error
		phisix    *phisixResponse
		phisixErr error
		mcap      *float64
		srcWG     sync.WaitGroup
	)
	srcWG.Add(2)
	go func() {
		defer srcWG.Done()
		reqCtx, cancel := boundCtx(cmd.Context(), flags)
		data, err := c.Get(reqCtx, stockDataPath, map[string]string{
			"cmpy_id":     strconv.Itoa(rc.CmpyID),
			"security_id": strconv.Itoa(rc.SecurityID),
		})
		cancel()
		if err != nil {
			edgeErr = err
			return
		}
		if s, err := pseedge.ParseStockData(string(data)); err != nil {
			edgeErr = err
		} else {
			snap = s
			mcap = pseedge.ParseMarketCap(string(data))
		}
	}()
	go func() {
		defer srcWG.Done()
		reqCtx, cancel := boundCtx(cmd.Context(), flags)
		data, err := c.Get(reqCtx, fmt.Sprintf(phisixStockURL, url.PathEscape(rc.Symbol)), nil)
		cancel()
		if err != nil {
			phisixErr = err
			return
		}
		var p phisixResponse
		if err := json.Unmarshal(data, &p); err != nil || len(p.Stocks) == 0 {
			phisixErr = fmt.Errorf("phisix %s: response has no stocks entry", rc.Symbol)
		} else {
			phisix = &p
		}
	}()
	srcWG.Wait()
	if mcap != nil {
		row.MarketCap = mcap
	}

	if snap == nil && phisix == nil {
		return nil, apiErr(fmt.Errorf("quote %s: both sources failed — edge: %v; phisix: %v", rc.Symbol, edgeErr, phisixErr))
	}

	switch {
	case snap != nil && phisix != nil:
		row.Source = "edge+phisix"
	case snap != nil:
		row.Source = "edge"
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: quote %s: phisix source failed (%v); serving edge only\n", rc.Symbol, phisixErr)
	default:
		row.Source = "phisix"
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: quote %s: edge source failed (%v); serving phisix only\n", rc.Symbol, edgeErr)
	}

	if phisix != nil {
		s := phisix.Stocks[0]
		if row.Name == "" {
			// Same issuer-string hardening as edge-parsed names: the phisix
			// mirror is community-operated, so its strings are untrusted too.
			row.Name = pseedge.CleanText(s.Name)
		}
		amount := s.Price.Amount
		row.Close = &amount
		pct := s.PercentChange
		row.ChangePct = &pct
		vol := s.Volume
		row.Volume = &vol
	}

	if snap != nil {
		if snap.Name != "" {
			row.Name = snap.Name
		}
		row.Open = snap.Open
		row.High = snap.High
		row.Low = snap.Low
		row.PrevClose = snap.PrevClose
		row.Value = snap.Value
		if row.Volume == nil {
			row.Volume = snap.Volume
		}

		// Cross-source close check: prefer edge, flag divergence > 0.01.
		if snap.LastTradedPrice != nil {
			if row.Close != nil && math.Abs(*row.Close-*snap.LastTradedPrice) > 0.01 {
				row.Divergence = true
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: quote %s: edge close %.4f vs phisix close %.4f diverge by more than 0.01; preferring edge\n",
					rc.Symbol, *snap.LastTradedPrice, *row.Close)
			}
			row.Close = snap.LastTradedPrice
		}

		if snap.Change != nil {
			row.Change = snap.Change
		}
		if snap.PctChange != nil {
			row.ChangePct = snap.PctChange
		}

		// Blank edge change cell = explicit closed-session state. (The
		// parser hard-errors on a non-blank cell it cannot match, so nil
		// here really does mean the cell was blank, not markup drift.)
		if snap.Change == nil && snap.PctChange == nil {
			row.Change = nil
			row.ChangePct = nil
			row.Note = "closed-session"
		}

		// The page's own As-of stamp predating the last completed session
		// (e.g. a suspended issue) also marks the row stale.
		if d, ok := parseEdgeAsOfDate(snap.AsOf); ok && d < state.LastCompleted {
			row.Stale = true
			if row.Note == "" {
				row.Note = "edge page as-of " + d + " predates last completed session"
			}
		}
	}

	if !state.TradingDay && row.Note == "" {
		row.Note = "closed-session"
	}
	return row, nil
}

// parseEdgeAsOfDate extracts the calendar date from a stockData.do "As of"
// stamp like "Jul 27, 2026 02:50 PM".
func parseEdgeAsOfDate(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parts := strings.SplitN(raw, " ", 4)
	if len(parts) < 3 {
		return "", false
	}
	if t, err := time.Parse("Jan 2, 2006", strings.Join(parts[:3], " ")); err == nil {
		return t.Format("2006-01-02"), true
	}
	return "", false
}
