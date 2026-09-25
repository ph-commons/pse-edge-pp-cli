// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored Daily Quotation Report command (issue #56). Registered from
// init so a reprint of root.go keeps the wiring.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
	"github.com/ph-commons/pse-edge-pp-cli/internal/quotationreport"
)

func init() {
	whichIndex = append(whichIndex, whichEntry{
		Command:      "quotation-report",
		Description:  "Official PSE Daily Quotation Report for one session: discover the End of Day Quotes PDF, retain the bytes, and return parsed rows with provenance. Not chart history.",
		Group:        "Downstream boundary",
		WhyItMatters: "Use this when a session must come from the official quotation report. history and export eod stay on DisclosureCht bars.",
	})
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newQuotationReportCmd(flags))
	})
}

func newQuotationReportCmd(flags *rootFlags) *cobra.Command {
	var dateFlag, symbolsFlag string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "quotation-report",
		Short: "Official Daily Quotation Report for one trading session",
		Long: `Discover, retain, and parse the PSE Daily Quotation Report for one session.

The listing page supplies the PDF link. A failed listing is an error. This
command does not invent a download URL and does not substitute the previous
session's report.

Rows use contract pse-edge-quotation-report-v1. They are not written into
chart history and they do not change pse-edge-export-eod-v1.

A dash is null (reported_dash). It is not a zero and it is not a suspension.
acquired_at is the only clock. Finding a dated PDF does not mean it was
published at 16:30.

PDF text requires Poppler pdftotext. The original PDF, its SHA-256, and the
discovery evidence are kept under the data directory. The same bytes are not
stored twice. A later file with a different hash is a revision and the earlier
PDF stays on disk.

PSE_QUOTATION_LISTING_URL overrides the listing page only. It is not a PDF URL.`,
		Example: `  pse-edge-pp-cli quotation-report --date 20260924 --json
  pse-edge-pp-cli quotation-report --date 2026-09-24 --symbols AT,DHI --agent
  pse-edge-pp-cli quotation-report --date 20260924 --refresh --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := parseSessionDate(dateFlag)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run quotation-report session=%s listing=%s\n", session.Format("2006-01-02"), listingURL())
				return nil
			}
			dataDir, err := cliutil.DataDir()
			if err != nil {
				return configErr(err)
			}
			symbols := splitCSV(symbolsFlag)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			var report quotationreport.Report
			if !refresh {
				loaded, ok, err := quotationreport.Open(dataDir, session, symbols)
				if err != nil {
					return quotationExit(err)
				}
				if ok {
					report = loaded
					return finishQuotation(cmd, flags, report)
				}
			}
			report, err = quotationreport.Fetch(ctx, dataDir, listingURL(), session, symbols, time.Now())
			if writeErr := writeQuotation(cmd.OutOrStdout(), flags, report); writeErr != nil {
				return writeErr
			}
			if err != nil {
				return quotationExit(err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dateFlag, "date", "", "Trading session, YYYYMMDD or YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&symbolsFlag, "symbols", "", "Comma-separated symbols to cover; omitted means the whole report")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Read the listing again. Same PDF bytes are reused. A new hash is kept as a revision.")
	_ = cmd.MarkFlagRequired("date")
	cmd.AddCommand(newQuotationReportQueryCmd(flags), newQuotationReportIndexCmd(flags))
	return cmd
}

func finishQuotation(cmd *cobra.Command, flags *rootFlags, report quotationreport.Report) error {
	if err := writeQuotation(cmd.OutOrStdout(), flags, report); err != nil {
		return err
	}
	if report.Status != quotationreport.StatusOK {
		return quotationExit(&quotationreport.Error{Status: report.Status, Detail: report.Detail})
	}
	return nil
}

func writeQuotation(w io.Writer, flags *rootFlags, report quotationreport.Report) error {
	if flags != nil && (flags.asJSON || flags.agent) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Fprintf(w, "session %s status %s rows %d\n", report.SessionDate, report.Status, report.Coverage.RowCount)
	if report.Source != nil {
		fmt.Fprintf(w, "sha256 %s\npdf %s\n", report.Source.SHA256, report.Source.PDFPath)
	}
	if report.Detail != "" {
		fmt.Fprintln(w, report.Detail)
	}
	fmt.Fprintln(w, "use --json for pse-edge-quotation-report-v1 rows")
	return nil
}

func listingURL() string {
	if v := strings.TrimSpace(os.Getenv("PSE_QUOTATION_LISTING_URL")); v != "" {
		return v
	}
	return quotationreport.DefaultListingURL
}

func parseSessionDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if when, err := time.Parse(layout, raw); err == nil {
			return time.Date(when.Year(), when.Month(), when.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid --date %q: expected YYYYMMDD or YYYY-MM-DD", raw)
}

func quotationExit(err error) error {
	qe, ok := err.(*quotationreport.Error)
	if !ok || qe == nil {
		return err
	}
	switch qe.Status {
	case quotationreport.StatusListingUnavailable, quotationreport.StatusReportUnavailable, quotationreport.StatusMissingSymbols:
		return notFoundErr(qe)
	default:
		return apiErr(qe)
	}
}
