// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored porcelain: symbol → cmpy_id/security_id resolution.
// Registered via the registerNovelCommand hook so generated root.go stays
// untouched. Also hosts resolvePSECompany, the shared resolution helper the
// quote/company/financials/filings porcelain reuses.

// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/pseedge"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/store"
)

const (
	autocompletePath = "/autoComplete/searchCompanyNameSymbol.ax"
	stockDataPath    = "/companyPage/stockData.do"
)

// resolvedCompany is the typed resolution result. Source is "local" when
// the registry answered, "edge" when the live autocomplete fallback did.
// Partial is true when the security_id recovery fetch failed and
// SecurityID is 0 — quotes for multi-class listings may hit the wrong
// series; Note says so.
type resolvedCompany struct {
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
	CmpyID     int    `json:"cmpy_id"`
	SecurityID int    `json:"security_id"`
	ETF        bool   `json:"etf"`
	Source     string `json:"source"`
	AsOf       string `json:"as_of"`
	Stale      bool   `json:"stale"`
	Partial    bool   `json:"partial,omitempty"`
	Note       string `json:"note,omitempty"`
}

// pseSymbolRE is the accepted ticker-symbol shape (after upper-casing):
// alphanumeric start, then up to 9 more alphanumerics, dots, or hyphens.
// Anything else is rejected before the symbol can reach a URL or query.
var pseSymbolRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.\-]{0,9}$`)

// validatePSESymbol normalizes (trim + upper-case) and validates a ticker
// symbol argument. Returns a usage-typed error on empty or malformed input.
func validatePSESymbol(symbol string) (string, error) {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" {
		return "", usageErr(fmt.Errorf("empty symbol"))
	}
	if !pseSymbolRE.MatchString(sym) {
		return "", usageErr(fmt.Errorf("invalid symbol %q: symbols are 1-10 characters of A-Z, 0-9, '.', '-' starting with a letter or digit", symbol))
	}
	return sym, nil
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newResolveCmd(flags))
	})
}

func newResolveCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "resolve <symbol>",
		Short: "Resolve a ticker symbol to its PSE Edge cmpy_id/security_id from the local registry",
		Long: `Use this command to turn a ticker symbol into the cmpy_id/security_id pair
every PSE Edge endpoint keys on. Do NOT use it for prices ('quote') or
company profiles ('company').

Resolution is exact-match against the local pse_companies registry
(symbols upper-cased). When the symbol is not in the registry, the live
autocomplete endpoint is tried with exact-symbol filtering — the
autocomplete serves only the first ~20 alphabetical prefix matches, so a
fuzzy first-row pick would routinely return the wrong issuer (AB must
resolve to Atok-Big Wedge, never AbaCore). A symbol absent from both is a
typed not-found (exit 3) with a 'sync market' hint.`,
		Example: `  pse-edge-pp-cli resolve GTCAP --json
  pse-edge-pp-cli resolve AB --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) != 1 {
				return usageErr(fmt.Errorf("resolve takes exactly one symbol, got %d", len(args)))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pse-edge-pp-cli")
			}
			rc, err := resolvePSECompany(cmd.Context(), cmd, flags, dbPath, args[0])
			if err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), rc, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// resolvePSECompany resolves one symbol: local registry first, live
// autocomplete (exact-symbol filter) as fallback. The fallback also fetches
// stockData.do to recover security_id, which autocomplete does not serve.
// Absent from both → typed not-found (exit 3) with a sync hint.
func resolvePSECompany(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath, symbol string) (*resolvedCompany, error) {
	sym, err := validatePSESymbol(symbol)
	if err != nil {
		return nil, err
	}
	state := psecal.SessionState(time.Now())
	asOf := state.LastCompleted
	stale := liveFetchStale(state)

	// Local registry. Missing-mirror guard: no database file means nothing
	// has ever been synced — hint, then fall through to the live fallback.
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: local store has not been synced yet. Run 'pse-edge-pp-cli sync market' to build the symbol registry; falling back to live autocomplete.")
	} else {
		db, err := store.OpenReadOnlyContext(ctx, dbPath)
		if err != nil {
			return nil, fmt.Errorf("opening local database: %w", err)
		}
		row, lookupErr := db.LookupPSECompanyBySymbol(ctx, sym)
		db.Close()
		if lookupErr == nil {
			return &resolvedCompany{
				Symbol:     row.Symbol,
				Name:       row.Name,
				CmpyID:     row.CmpyID,
				SecurityID: row.SecurityID,
				ETF:        row.ETF,
				Source:     "local",
				AsOf:       asOf,
				Stale:      stale,
			}, nil
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			// A half-initialized store (interrupted first sync) has the DB
			// file but not the registry table. That is an unsynced state,
			// not a fatal error — hint and fall through to live autocomplete.
			if strings.Contains(lookupErr.Error(), "no such table") {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: local registry is missing (interrupted sync?). Run 'pse-edge-pp-cli sync market'; falling back to live autocomplete.")
			} else {
				return nil, lookupErr
			}
		}
	}

	// Live fallback: autocomplete with EXACT symbol match only.
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	c.NoCache = true
	reqCtx, cancel := boundCtx(ctx, flags)
	data, err := c.Get(reqCtx, autocompletePath, map[string]string{"term": sym})
	cancel()
	if err != nil {
		return nil, apiErr(fmt.Errorf("symbol %s not in local registry and autocomplete fallback failed: %w\nhint: run 'pse-edge-pp-cli sync market' to build the registry", sym, err))
	}
	matches, err := pseedge.ParseAutocomplete(data)
	if err != nil {
		return nil, apiErr(err)
	}
	for _, m := range matches {
		if !strings.EqualFold(m.Symbol, sym) {
			continue
		}
		rc := &resolvedCompany{
			Symbol: strings.ToUpper(m.Symbol),
			Name:   m.Name,
			CmpyID: m.CmpyID,
			ETF:    m.ETF,
			Source: "edge",
			AsOf:   asOf,
			Stale:  stale,
		}
		// Autocomplete has no security_id; recover it from stockData.do.
		// A failed recovery is surfaced, never silent: SecurityID 0 sends
		// multi-class listings (e.g. preferred series) to the wrong series.
		var recoverErr error
		snapCtx, snapCancel := boundCtx(ctx, flags)
		page, snapErr := c.Get(snapCtx, stockDataPath, map[string]string{"cmpy_id": fmt.Sprintf("%d", m.CmpyID)})
		snapCancel()
		if snapErr != nil {
			recoverErr = snapErr
		} else if snap, parseErr := pseedge.ParseStockData(string(page)); parseErr != nil {
			recoverErr = parseErr
		} else {
			rc.SecurityID = snap.SecurityID
			if snap.Name != "" {
				rc.Name = snap.Name
			}
		}
		if rc.SecurityID == 0 {
			rc.Partial = true
			rc.Note = "resolved without security_id — quotes for multi-class listings may hit the wrong series"
			if recoverErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: resolve %s: security_id recovery from stockData.do failed (%v); %s\n", rc.Symbol, recoverErr, rc.Note)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: resolve %s: stockData.do served no security_id; %s\n", rc.Symbol, rc.Note)
			}
		}
		return rc, nil
	}
	return nil, notFoundErr(fmt.Errorf("symbol %q not found: not in the local registry and no exact match on PSE Edge autocomplete\nhint: run 'pse-edge-pp-cli sync market' first, then retry; check the ticker spelling on edge.pse.com.ph", sym))
}
