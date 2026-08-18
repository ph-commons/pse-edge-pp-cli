// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored porcelain: typed 17-A/17-Q financial summary tables by
// ticker symbol.
//
// Naming note: the generated promoted endpoint command also claims the
// top-level name "financials" (raw HTML extraction keyed by --company-id).
// Per the absorb manifest this porcelain owns the name; the hook below
// re-parents the generated command as the "report" subcommand — exactly
// what its own generated Example ("pse-edge-pp-cli financials report
// --company-id 633") already documents — so no generated functionality is
// lost and no generated file is edited.

// pp:data-source live

package cli

import (
	"errors"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/pseedge"
)

const financialReportsPath = "/companyPage/financial_reports_view.do"

// financialsOut is the typed financial-reports payload keyed by symbol.
type financialsOut struct {
	Symbol    string                    `json:"symbol"`
	CmpyID    int                       `json:"cmpy_id"`
	Annual    *pseedge.FinancialSection `json:"annual"`
	Quarterly *pseedge.FinancialSection `json:"quarterly"`
	AsOf      string                    `json:"as_of"`
	Stale     bool                      `json:"stale"`
	Source    string                    `json:"source"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		porcelain := newFinancialsCmd(flags)
		for _, c := range root.Commands() {
			if c.Name() == "financials" {
				root.RemoveCommand(c)
				c.Use = "report"
				c.Short = "Raw financial-reports page extraction (generated endpoint mirror keyed by --company-id)"
				porcelain.AddCommand(c)
				break
			}
		}
		root.AddCommand(porcelain)
	})
}

func newFinancialsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "financials <symbol>",
		Short: "Typed 17-A/17-Q summary tables by ticker: balance sheet and income statement, annual and quarterly, parsed to floats",
		Long: `Use this command for a company's filed financial summary tables
(Current/Total Assets and Liabilities, Retained Earnings, Stockholders'
Equity, Book Value Per Share, Gross Revenue/Expense, Net Income, EPS) as
typed JSON — annual and quarterly sections with current and prior columns.
Do NOT use it for the raw disclosure documents ('filings' porcelain or the
'disclosures' resource commands). The raw generated page extraction
remains available as 'financials report --company-id N'.

Values are parsed as floats (comma-stripped; parenthesized = negative) and
also preserved as printed — units are whatever the company filed in (the
section's units field says which). A company with no statements published
on Edge is a typed "no-financials-published" error, never empty success.`,
		Example:     "  pse-edge-pp-cli financials GTCAP --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pse-edge-pp-cli")
			}
			rc, err := resolvePSECompany(cmd.Context(), cmd, flags, dbPath, args[0])
			if err != nil {
				return err
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			cmpyID := strconv.Itoa(rc.CmpyID)
			reqCtx, cancel := boundCtx(cmd.Context(), flags)
			data, err := c.Get(reqCtx, financialReportsPath, map[string]string{"cmpy_id": cmpyID})
			cancel()
			if err != nil {
				return classifyAPIError(err, flags)
			}
			report, err := pseedge.ParseFinancialReports(string(data), cmpyID)
			if err != nil {
				var nfe *pseedge.NoFinancialsError
				if errors.As(err, &nfe) {
					return notFoundErr(err)
				}
				return apiErr(err)
			}

			state := psecal.SessionState(time.Now())
			out := financialsOut{
				Symbol:    rc.Symbol,
				CmpyID:    rc.CmpyID,
				Annual:    report.Annual,
				Quarterly: report.Quarterly,
				AsOf:      state.LastCompleted,
				Stale:     liveFetchStale(state),
				Source:    "edge",
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path for symbol resolution (default: resolved data directory data.db)")
	return cmd
}
