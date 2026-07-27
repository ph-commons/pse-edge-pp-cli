// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored PSE Edge market sync, wired as a subcommand of the
// generated `sync` command via the registerNovelCommand hook so the
// generated sync.go stays untouched (and generic-resource sync keeps
// working). Invocation: `pse-edge-pp-cli sync market [--symbols ...]`.

// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/client"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/cliutil"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/pseedge"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/store"
)

const (
	directoryPageURL   = "https://edge.pse.com.ph/companyDirectory/search.ax"
	compositeSectorURL = "https://frames.pse.com.ph/compositeSector"
	// dogfoodMaxDirectoryPages caps the registry crawl under the
	// live-dogfood runner so its flat per-command timeout holds.
	dogfoodMaxDirectoryPages = 2
	// maxDirectoryPages is the hard safety cap on the registry crawl in ALL
	// environments: 282 listed companies ≈ 6 pages, so 40 is generous; the
	// cap guards against an upstream that repeats the last page forever.
	maxDirectoryPages = 40
	// dogfoodMaxHistoryWindow caps the per-symbol history window under
	// the live-dogfood runner.
	dogfoodMaxHistoryWindow = 30 * 24 * time.Hour
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "sync" {
				marketCmd := newSyncMarketCmd(flags)
				c.AddCommand(marketCmd)
				// The spec's resources are HTML/param-gated, so the generated
				// generic sync has no default resources and a bare `sync` is a
				// runtime no-op. Delegate the bare invocation (no flags, no
				// args) to the real population path, `sync market`, so
				// store-dependent commands always have an advertised sync.
				origRunE := c.RunE
				c.RunE = func(cmd *cobra.Command, args []string) error {
					if len(args) == 0 && cmd.Flags().NFlag() == 0 {
						fmt.Fprintln(cmd.ErrOrStderr(), "sync: no resources specified — running 'sync market' (registry + index + composite)")
						// Pass the live outer cmd: marketCmd never went through
						// Execute, so its own context is nil (store open would
						// deadlock). The closure only uses cmd for
						// Context()/writers; its flag vars keep their defaults.
						return marketCmd.RunE(cmd, nil)
					}
					if origRunE != nil {
						return origRunE(cmd, args)
					}
					return cmd.Help()
				}
				return
			}
		}
	})
}

func newSyncMarketCmd(flags *rootFlags) *cobra.Command {
	var symbolsFlag []string
	var sinceFlag string
	var dbPath string
	var skipCompanies bool
	var skipIndex bool

	cmd := &cobra.Command{
		Use:   "market",
		Short: "Sync PSE Edge market data: company registry, PSEi/sector index snapshots (+ one-time series backfill), and per-ticker EOD bars",
		Long: `Populate the hand-authored PSE Edge tables from edge.pse.com.ph and
frames.pse.com.ph (public endpoints, no auth):

  pse_companies        full paginated company directory (stops on first empty page)
  pse_index_snapshots  embedded daily PSEi series backfill (idempotent) plus the
                       current composite reading — written ONLY when the session
                       is post-close per the trading calendar, otherwise skipped
                       with a note (EOD snapshots must be final data)
  pse_eod_prices       per-ticker daily bars from DisclosureCht.ax over --since
                       (rows are inherently final history); every row passes the
                       price-sanity gate before it can land

Symbols come from --symbols; without the flag, every company already in
pse_companies is synced (a full registry crawl runs first when the registry
is empty). Progress is emitted as NDJSON events on stdout, matching the
generated sync command's event stream.`,
		Example: `  # Registry + index + 14 days of AT bars
  pse-edge-pp-cli sync market --symbols AT --since 14d

  # Everything already registered, default 30d window
  pse-edge-pp-cli sync market`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			window, err := parseSinceWindow(sinceFlag, 30*24*time.Hour)
			if err != nil {
				return err
			}
			if cliutil.IsDogfoodEnv() && window > dogfoodMaxHistoryWindow {
				window = dogfoodMaxHistoryWindow
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true

			if dbPath == "" {
				dbPath = defaultDBPath("pse-edge-pp-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if err := db.EnsurePSEEdgeTables(cmd.Context()); err != nil {
				return err
			}

			events := cmd.OutOrStdout()
			symbols := splitSymbols(symbolsFlag)
			var failures []string

			// Phase 1: company registry. Skipped by flag, or when a
			// --symbols run can already resolve every symbol locally.
			needRegistry := !skipCompanies
			if needRegistry && len(symbols) > 0 {
				if allSymbolsResolvable(cmd.Context(), db, symbols) {
					needRegistry = false
				}
			}
			if needRegistry {
				count, err := syncMarketCompanies(cmd.Context(), c, db, flags, events)
				if err != nil {
					failures = append(failures, fmt.Sprintf("pse_companies: %v", err))
					marketEvent(events, map[string]any{"event": "sync_error", "resource": "pse_companies", "error": err.Error()})
				} else {
					_ = db.SaveSyncState("pse_companies", "", count)
					marketEvent(events, map[string]any{"event": "sync_complete", "resource": "pse_companies", "count": count})
				}
			}

			// Phase 2: index snapshots (series backfill + current reading).
			if !skipIndex {
				count, err := syncMarketIndex(cmd.Context(), c, db, flags, events)
				if err != nil {
					failures = append(failures, fmt.Sprintf("pse_index_snapshots: %v", err))
					marketEvent(events, map[string]any{"event": "sync_error", "resource": "pse_index_snapshots", "error": err.Error()})
				} else {
					_ = db.SaveSyncState("pse_index_snapshots", "", count)
					marketEvent(events, map[string]any{"event": "sync_complete", "resource": "pse_index_snapshots", "count": count})
				}
			}

			// Phase 3: per-ticker EOD bars.
			if len(symbols) == 0 {
				all, err := db.ListPSECompanySymbols(cmd.Context())
				if err != nil {
					return err
				}
				symbols = all
			}
			eodCount, eodFailures := syncMarketEOD(cmd.Context(), c, db, flags, events, symbols, window)
			failures = append(failures, eodFailures...)
			if eodCount > 0 {
				_ = db.SaveSyncState("pse_eod_prices", "", eodCount)
			}
			marketEvent(events, map[string]any{
				"event": "sync_summary", "resource": "market",
				"eod_rows": eodCount, "symbols": len(symbols), "failed": len(failures),
			})

			if len(failures) > 0 {
				if eodCount == 0 && len(failures) >= len(symbols) && len(symbols) > 0 {
					return fmt.Errorf("market sync failed: %s", strings.Join(failures, "; "))
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: market sync completed with %d failure(s): %s\n", len(failures), strings.Join(failures, "; "))
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&symbolsFlag, "symbols", nil, "Ticker symbols to sync EOD bars for (comma-separated; default: all companies in pse_companies)")
	cmd.Flags().StringVar(&sinceFlag, "since", "30d", "History window for EOD bars (e.g. 14d, 4w; capped at 30d under the dogfood runner)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().BoolVar(&skipCompanies, "skip-companies", false, "Skip the company registry crawl")
	cmd.Flags().BoolVar(&skipIndex, "skip-index", false, "Skip the composite/index snapshot fetch")
	return cmd
}

// splitSymbols normalizes a repeated/comma-joined --symbols flag.
func splitSymbols(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			part = strings.ToUpper(strings.TrimSpace(part))
			if part != "" && !seen[part] {
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	sort.Strings(out)
	return out
}

// allSymbolsResolvable reports whether every requested symbol already has
// a registry row, letting a --symbols run skip the full directory crawl.
func allSymbolsResolvable(ctx context.Context, db *store.Store, symbols []string) bool {
	for _, sym := range symbols {
		if _, err := db.LookupPSECompanyBySymbol(ctx, sym); err != nil {
			return false
		}
	}
	return true
}

func marketEvent(w io.Writer, payload map[string]any) {
	if humanFriendly {
		return
	}
	line, _ := json.Marshal(payload)
	fmt.Fprintf(w, "%s\n", line)
}

// syncMarketCompanies crawls the paginated company directory until the
// first empty page (or the dogfood page cap) and upserts every row.
func syncMarketCompanies(ctx context.Context, c *client.Client, db *store.Store, flags *rootFlags, events io.Writer) (int, error) {
	marketEvent(events, map[string]any{"event": "sync_start", "resource": "pse_companies"})
	total := 0
	for page := 1; ; page++ {
		reqCtx, cancel := boundCtx(ctx, flags)
		data, err := c.Get(reqCtx, directoryPageURL, map[string]string{"pageNo": strconv.Itoa(page)})
		cancel()
		if err != nil {
			return total, fmt.Errorf("directory page %d: %w", page, err)
		}
		companies, err := pseedge.ParseDirectoryPage(string(data), page)
		if err != nil {
			return total, fmt.Errorf("directory page %d: %w", page, err)
		}
		if len(companies) == 0 {
			break
		}
		rows := make([]store.PSECompanyRow, 0, len(companies))
		for _, co := range companies {
			rows = append(rows, store.PSECompanyRow{
				CmpyID:     co.CmpyID,
				SecurityID: co.SecurityID,
				Symbol:     co.Symbol,
				Name:       co.Name,
			})
		}
		if err := db.UpsertPSECompanies(ctx, rows); err != nil {
			return total, err
		}
		total += len(rows)
		marketEvent(events, map[string]any{"event": "page_fetch", "resource": "pse_companies", "page": page, "rows": len(rows)})
		if cliutil.IsDogfoodEnv() && page >= dogfoodMaxDirectoryPages {
			marketEvent(events, map[string]any{"event": "sync_warning", "resource": "pse_companies", "reason": "dogfood_page_cap", "message": "registry crawl capped at 2 pages under the dogfood runner"})
			break
		}
		if page >= maxDirectoryPages {
			marketEvent(events, map[string]any{"event": "sync_warning", "resource": "pse_companies", "reason": "directory_page_cap", "message": fmt.Sprintf("registry crawl stopped at the %d-page safety cap without seeing an empty page — upstream pagination may be misbehaving", maxDirectoryPages)})
			break
		}
	}
	return total, nil
}

// syncMarketIndex fetches compositeSector once, backfills the embedded
// daily PSEi series (idempotent upsert), and writes the current composite
// reading only when the session is post-close per psecal — provisional
// intraday readings never land in the EOD snapshot table.
func syncMarketIndex(ctx context.Context, c *client.Client, db *store.Store, flags *rootFlags, events io.Writer) (int, error) {
	marketEvent(events, map[string]any{"event": "sync_start", "resource": "pse_index_snapshots"})
	reqCtx, cancel := boundCtx(ctx, flags)
	data, err := c.Get(reqCtx, compositeSectorURL, nil)
	cancel()
	if err != nil {
		return 0, fmt.Errorf("compositeSector: %w", err)
	}
	body := string(data)

	comp, err := pseedge.ParseComposite(body)
	if err != nil {
		return 0, err
	}
	series, err := pseedge.ParsePSEISeries(body)
	if err != nil {
		return 0, err
	}

	total := 0

	// One-time (idempotent) PSEi series backfill. OHLC-only rows: the
	// series close is stored as value; change/breadth stay NULL-ish.
	backfill := make([]store.PSEIndexSnapshotRow, 0, len(series))
	for _, p := range series {
		backfill = append(backfill, store.PSEIndexSnapshotRow{
			IndexCode:   "PSEI",
			TradingDate: p.Date,
			Value:       p.Close,
			Source:      "edge",
		})
	}
	if err := db.UpsertPSEIndexSnapshots(ctx, backfill); err != nil {
		return 0, fmt.Errorf("PSEi series backfill: %w", err)
	}
	total += len(backfill)
	marketEvent(events, map[string]any{"event": "backfill", "resource": "pse_index_snapshots", "index_code": "PSEI", "rows": len(backfill)})

	// Current composite reading: only a completed session may be written,
	// and only under the date the page itself reports. If the page's own
	// trade date (Manila date part) does not equal the calendar's last
	// completed session, writing it under state.LastCompleted would misdate
	// the snapshot — skip with a warning instead.
	state := psecal.SessionState(time.Now())
	if state.Phase == "post-close" {
		tradingDate := state.LastCompleted
		if pageDate, raw := compositeTradeDate(comp); pageDate != tradingDate {
			marketEvent(events, map[string]any{
				"event": "sync_warning", "resource": "pse_index_snapshots",
				"reason":  "trade_date_mismatch",
				"message": fmt.Sprintf("composite page trade date %q (raw %q) != last completed session %s; snapshot skipped rather than written under the wrong date", pageDate, raw, tradingDate),
			})
			return total, nil
		}
		rows := make([]store.PSEIndexSnapshotRow, 0, len(comp.Indices))
		for _, idx := range comp.Indices {
			change := idx.Change
			pct := idx.PctChange
			row := store.PSEIndexSnapshotRow{
				IndexCode:   idx.Code,
				TradingDate: tradingDate,
				Value:       idx.Value,
				Change:      &change,
				PctChange:   &pct,
				Source:      "edge",
			}
			if idx.Code == "PSEI" {
				row.Advances = comp.Advances
				row.Declines = comp.Declines
				row.Unchanged = comp.Unchanged
				row.TotalVolume = comp.TotalVolume
				row.TotalValue = comp.TotalValue
				row.TotalTrades = comp.TotalTrades
			}
			rows = append(rows, row)
		}
		if err := db.UpsertPSEIndexSnapshots(ctx, rows); err != nil {
			return total, fmt.Errorf("composite snapshot: %w", err)
		}
		total += len(rows)
		marketEvent(events, map[string]any{"event": "snapshot", "resource": "pse_index_snapshots", "trading_date": tradingDate, "indices": len(rows)})
	} else {
		marketEvent(events, map[string]any{
			"event": "sync_warning", "resource": "pse_index_snapshots",
			"reason":  "session_not_post_close",
			"message": fmt.Sprintf("session phase is %q; composite snapshot skipped — only final post-close data is written (series backfill still ran)", state.Phase),
		})
	}
	return total, nil
}

// compositeTradeDate extracts the composite page's own trade date (Manila
// calendar date) from the PSEI reading's ISO trade stamp. Returns the date
// (or "" when absent/unparseable) plus the raw stamp for diagnostics.
func compositeTradeDate(comp *pseedge.Composite) (string, string) {
	for _, idx := range comp.Indices {
		if idx.Code != "PSEI" {
			continue
		}
		raw := strings.TrimSpace(idx.TradeDate)
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.In(psecal.Manila()).Format("2006-01-02"), raw
		}
		// Offset-less renders: the stamp is already Manila wall time.
		if len(raw) >= 10 {
			if _, err := time.Parse("2006-01-02", raw[:10]); err == nil {
				return raw[:10], raw
			}
		}
		return "", raw
	}
	return "", ""
}

// syncMarketEOD fetches DisclosureCht.ax history per symbol, validates
// every row through the price-sanity gate, and upserts survivors. The
// fetch window is clamped to the last COMPLETED trading day — an
// in-progress session's provisional bar must never land in pse_eod_prices
// (rows there are final by contract); any bar the endpoint still returns
// past that day is skipped and counted as provisional_skipped.
// Per-symbol failures are collected, not fatal.
func syncMarketEOD(ctx context.Context, c *client.Client, db *store.Store, flags *rootFlags, events io.Writer, symbols []string, window time.Duration) (int, []string) {
	marketEvent(events, map[string]any{"event": "sync_start", "resource": "pse_eod_prices", "symbols": len(symbols)})
	end := psecal.LastCompletedTradingDay(time.Now())
	lastCompleted := end.Format("2006-01-02")
	start := end.Add(-window)
	total := 0
	var failures []string
	for _, sym := range symbols {
		co, err := db.LookupPSECompanyBySymbol(ctx, sym)
		if err != nil {
			reason := "not in pse_companies (run 'sync market' without --skip-companies first)"
			if !errors.Is(err, sql.ErrNoRows) {
				reason = err.Error()
			}
			failures = append(failures, fmt.Sprintf("%s: %s", sym, reason))
			marketEvent(events, map[string]any{"event": "sync_warning", "resource": "pse_eod_prices", "symbol": sym, "reason": "symbol_unresolved", "message": reason})
			continue
		}
		reqCtx, cancel := boundCtx(ctx, flags)
		bars, err := pseedge.FetchHistory(reqCtx, c, co.CmpyID, co.SecurityID, start, end)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", sym, err))
			marketEvent(events, map[string]any{"event": "sync_warning", "resource": "pse_eod_prices", "symbol": sym, "reason": "fetch_failed", "message": err.Error()})
			continue
		}
		rows := make([]store.PSEEODRow, 0, len(bars))
		rejected := 0
		provisionalSkipped := 0
		for _, bar := range bars {
			if err := pseedge.ValidateEOD(bar); err != nil {
				rejected++
				marketEvent(events, map[string]any{"event": "sync_warning", "resource": "pse_eod_prices", "symbol": sym, "reason": "row_rejected", "message": err.Error()})
				continue
			}
			// Belt over the clamped window: a bar dated after the last
			// completed session is provisional — never stored.
			if bar.TradingDate > lastCompleted {
				provisionalSkipped++
				continue
			}
			rows = append(rows, store.PSEEODRow{
				Symbol:      sym,
				TradingDate: bar.TradingDate,
				Open:        bar.Open,
				High:        bar.High,
				Low:         bar.Low,
				Close:       bar.Close,
				Value:       bar.Value,
				Source:      "edge",
			})
		}
		if err := db.UpsertPSEEODPrices(ctx, rows); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", sym, err))
			continue
		}
		total += len(rows)
		marketEvent(events, map[string]any{"event": "symbol_synced", "resource": "pse_eod_prices", "symbol": sym, "rows": len(rows), "rejected": rejected, "provisional_skipped": provisionalSkipped})
	}
	return total, failures
}
