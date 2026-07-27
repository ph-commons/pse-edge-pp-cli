// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored porcelain: typed company profile by ticker symbol.
// Registered via the registerNovelCommand hook.

// pp:data-source live

package cli

import (
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/pseedge"
)

const companyProfilePath = "/companyInformation/form.do"

// companyOut is the typed company profile keyed by symbol.
type companyOut struct {
	Symbol            string `json:"symbol"`
	Name              string `json:"name"`
	CmpyID            int    `json:"cmpy_id"`
	SecurityID        int    `json:"security_id"`
	Sector            string `json:"sector"`
	Subsector         string `json:"subsector"`
	IncorporationDate string `json:"incorporation_date,omitempty"`
	FiscalYear        string `json:"fiscal_year,omitempty"`
	Auditor           string `json:"auditor,omitempty"`
	CorporateLife     string `json:"corporate_life,omitempty"`
	Directors         *int   `json:"directors"`
	Website           string `json:"website,omitempty"`
	AsOf              string `json:"as_of"`
	Stale             bool   `json:"stale"`
	Source            string `json:"source"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newCompanyCmd(flags))
	})
}

func newCompanyCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "company <symbol>",
		Short: "Typed company profile by ticker: sector, subsector, incorporation date, fiscal year, auditor, directors",
		Long: `Use this command for a company's profile (sector, subsector,
incorporation date, fiscal year, external auditor, corporate life, number
of directors) resolved by ticker symbol. Do NOT use it for prices
('quote') or financial statements ('financials').

The symbol is resolved to cmpy_id/security_id via the local registry (live
autocomplete fallback), then companyInformation/form.do is fetched and
parsed to typed JSON. Bogus symbols and challenge pages are typed hard
errors, never empty output.`,
		Example:     "  pse-edge-pp-cli company GTCAP --json",
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
			params := map[string]string{"cmpy_id": strconv.Itoa(rc.CmpyID)}
			if rc.SecurityID > 0 {
				params["security_id"] = strconv.Itoa(rc.SecurityID)
			}
			reqCtx, cancel := boundCtx(cmd.Context(), flags)
			data, err := c.Get(reqCtx, companyProfilePath, params)
			cancel()
			if err != nil {
				return classifyAPIError(err, flags)
			}
			profile, err := pseedge.ParseCompanyProfile(string(data))
			if err != nil {
				return apiErr(err)
			}

			state := psecal.SessionState(time.Now())
			out := companyOut{
				Symbol:            rc.Symbol,
				Name:              profile.Name,
				CmpyID:            rc.CmpyID,
				SecurityID:        rc.SecurityID,
				Sector:            profile.Sector,
				Subsector:         profile.Subsector,
				IncorporationDate: profile.IncorporationDate,
				FiscalYear:        profile.FiscalYear,
				Auditor:           profile.Auditor,
				CorporateLife:     profile.CorporateLife,
				Directors:         profile.Directors,
				Website:           profile.Website,
				AsOf:              state.LastCompleted,
				Stale:             liveFetchStale(state),
				Source:            "edge",
			}
			if out.Name == "" {
				out.Name = rc.Name
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path for symbol resolution (default: resolved data directory data.db)")
	return cmd
}
