// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored porcelain: typed PSEi/market snapshot from compositeSector.
//
// Naming note: the generated promoted endpoint command also claims the
// top-level name "market" (raw HTML extraction of the same page). Per the
// absorb manifest this porcelain owns the name; the hook below re-parents
// the generated command as the "composite" subcommand — exactly what its
// own generated Example ("pse-edge-pp-cli market composite") already
// documents — so no generated functionality is lost and no generated file
// is edited.

// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/pseedge"
)

// marketIndexOut is one index reading (PSEi or a sector).
type marketIndexOut struct {
	Code   string  `json:"code,omitempty"`
	Name   string  `json:"name,omitempty"`
	Value  float64 `json:"value"`
	Change float64 `json:"change"`
	Pct    float64 `json:"change_pct"`
}

type marketBreadthOut struct {
	Advances  *int64 `json:"advances"`
	Declines  *int64 `json:"declines"`
	Unchanged *int64 `json:"unchanged"`
}

type marketTotalsOut struct {
	Volume *float64 `json:"volume"`
	Value  *float64 `json:"value"`
	Trades *int64   `json:"trades"`
}

// marketOut is the typed snapshot. AsOf is ALWAYS the last completed
// trading day per the calendar (the binding as-of invariant, same as
// quote); SessionDate is the page's own trade date, kept separate so a
// lagging page can never masquerade as the completed session.
type marketOut struct {
	PSEI        marketIndexOut   `json:"psei"`
	Sectors     []marketIndexOut `json:"sectors,omitempty"`
	Breadth     marketBreadthOut `json:"breadth"`
	Totals      marketTotalsOut  `json:"totals"`
	AsOf        string           `json:"as_of"`
	SessionDate string           `json:"session_date,omitempty"`
	Stale       bool             `json:"stale"`
	Source      string           `json:"source"`
	Note        string           `json:"note,omitempty"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		porcelain := newMarketCmd(flags)
		for _, c := range root.Commands() {
			if c.Name() == "market" {
				root.RemoveCommand(c)
				c.Use = "composite"
				c.Short = "Raw composite/sector page extraction (generated endpoint mirror of frames.pse.com.ph/compositeSector)"
				porcelain.AddCommand(c)
				break
			}
		}
		root.AddCommand(porcelain)
	})
}

func newMarketCmd(flags *rootFlags) *cobra.Command {
	var withSectors bool

	cmd := &cobra.Command{
		Use:   "market",
		Short: "Typed market snapshot: PSEi level/change, breadth (advances/declines/unchanged), and totals, session-state aware",
		Long: `Use this command for today's PSEi level, market breadth, and traded
totals as typed JSON. Do NOT use it for breadth over time ('breadth') or
per-ticker prices ('quote'). The raw generated page extraction remains
available as 'market composite'.

Data comes from frames.pse.com.ph/compositeSector (PSEi + sector readings
in entity-encoded hidden inputs, summary table for breadth and totals).
Post-close on a trading day the snapshot is final (stale:false); before
the 16:00 Asia/Manila close gate — or on a non-trading day — it is marked
stale:true with a note. Challenge or shell pages are typed hard errors,
never empty output.`,
		Example: `  pse-edge-pp-cli market --json
  pse-edge-pp-cli market --sectors --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true

			reqCtx, cancel := boundCtx(cmd.Context(), flags)
			data, err := c.Get(reqCtx, compositeSectorURL, nil)
			cancel()
			if err != nil {
				return classifyAPIError(err, flags)
			}
			comp, err := pseedge.ParseComposite(string(data))
			if err != nil {
				return apiErr(err)
			}

			out := marketOut{Source: "edge"}
			for _, idx := range comp.Indices {
				entry := marketIndexOut{Code: idx.Code, Value: idx.Value, Change: idx.Change, Pct: idx.PctChange}
				if idx.Code == "PSEI" {
					entry.Code = ""
					out.PSEI = entry
					out.SessionDate = tradeDateOnly(idx.TradeDate)
				} else if withSectors {
					out.Sectors = append(out.Sectors, entry)
				}
			}
			if out.PSEI.Value == 0 {
				return apiErr(fmt.Errorf("compositeSector: PSEI reading missing or zero; refusing to emit empty market data"))
			}
			out.Breadth = marketBreadthOut{Advances: comp.Advances, Declines: comp.Declines, Unchanged: comp.Unchanged}
			out.Totals = marketTotalsOut{Volume: comp.TotalVolume, Value: comp.TotalValue, Trades: comp.TotalTrades}

			// as_of is pinned to the calendar's last completed session (the
			// binding invariant, same as quote); the page's own trade date
			// stays in session_date and only drives the stale/note verdict.
			state := psecal.SessionState(time.Now())
			lastCompleted := state.LastCompleted
			out.AsOf = lastCompleted
			switch {
			case out.SessionDate != "" && out.SessionDate < lastCompleted:
				out.Stale = true
				out.Note = fmt.Sprintf("page trade date %s predates last completed session %s", out.SessionDate, lastCompleted)
			case state.TradingDay && state.Phase != "post-close":
				out.Stale = true
				out.Note = fmt.Sprintf("provisional: session phase is %q; figures may change until the 16:00 Asia/Manila close", state.Phase)
			case !state.TradingDay:
				out.Stale = true
				out.Note = "non-trading day: showing last completed session " + lastCompleted
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().BoolVar(&withSectors, "sectors", false, "Include per-sector index readings (default: PSEi only)")
	return cmd
}

// tradeDateOnly reduces an ISO trade stamp ("2026-07-27T14:50:00+08:00")
// to its calendar date; empty when unparseable.
func tradeDateOnly(iso string) string {
	iso = strings.TrimSpace(iso)
	if len(iso) >= 10 {
		if _, err := time.Parse("2006-01-02", iso[:10]); err == nil {
			return iso[:10]
		}
	}
	return ""
}
