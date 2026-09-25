// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
	"github.com/ph-commons/pse-edge-pp-cli/internal/quotationreport"
	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

const (
	quotationRowsContract  = "pse-edge-quotation-rows-v1"
	quotationIndexContract = "pse-edge-quotation-index-v1"
)

func init() {
	whichIndex = append(whichIndex,
		whichEntry{
			Command:      "quotation-report query",
			Description:  "Query admitted Daily Quotation Report rows across sessions. SQLite in data.db is internal. history and export eod are unchanged.",
			Group:        "Downstream boundary",
			WhyItMatters: "Read retained quotation rows, including an older revision, without treating them as chart history.",
		},
		whichEntry{
			Command:      "quotation-report index",
			Description:  "Load quotation revisions already named by index.json into internal SQLite. Does not download PDFs. history and export eod are unchanged.",
			Group:        "Downstream boundary",
			WhyItMatters: "Rebuild the internal quotation tables from retained JSON without a refetch.",
		},
	)
}

func newQuotationReportQueryCmd(flags *rootFlags) *cobra.Command {
	var dateFlag, fromFlag, toFlag, symbolsFlag, shaFlag, dbFlag string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query admitted quotation rows",
		Long: `Read admitted Daily Quotation Report rows from the local store.

The JSON contract is pse-edge-quotation-rows-v1. data.db tables are internal.
history and export eod are unchanged. A dash is null reported_dash, not zero.
A missing session is omitted. It is not a zero close.

Pass --date, or both --from and --to. --sha selects one complete revision,
including a retained previous hash.`,
		Example: `  pse-edge-pp-cli quotation-report query --date 20260924 --json
  pse-edge-pp-cli quotation-report query --from 20260924 --to 20260925 --symbols AT,DHI --json
  pse-edge-pp-cli quotation-report query --date 20260924 --sha <hash> --json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run quotation-report query date=%s from=%s to=%s sha=%s\n", dateFlag, fromFlag, toFlag, shaFlag)
				return nil
			}
			from, to, err := quotationRange(dateFlag, fromFlag, toFlag)
			if err != nil {
				return usageErr(err)
			}
			dataDir, err := cliutil.DataDir()
			if err != nil {
				return configErr(err)
			}
			dbPath := quotationDBPath(dbFlag)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			sha := strings.TrimSpace(shaFlag)
			if sha == "" {
				if err := quotationreport.CheckQuotationCurrentRange(ctx, dataDir, dbPath, from, to); err != nil {
					return err
				}
			}
			payload, err := loadQuotationRows(ctx, dbPath, from, to, sha, splitCSV(symbolsFlag))
			if err != nil {
				if errors.Is(err, store.ErrQuotationRevisionNotFound) {
					return notFoundErr(err)
				}
				return err
			}
			return writeQuotationRows(cmd.OutOrStdout(), flags, payload)
		},
	}
	cmd.Flags().StringVar(&dateFlag, "date", "", "One session, YYYYMMDD or YYYY-MM-DD")
	cmd.Flags().StringVar(&fromFlag, "from", "", "First session, inclusive, with --to")
	cmd.Flags().StringVar(&toFlag, "to", "", "Last session, inclusive, with --from")
	cmd.Flags().StringVar(&symbolsFlag, "symbols", "", "Comma-separated symbols to cover")
	cmd.Flags().StringVar(&shaFlag, "sha", "", "One complete revision, including a previous hash")
	cmd.Flags().StringVar(&dbFlag, "db", "", "SQLite database path (default: data dir data.db)")
	return cmd
}

func newQuotationReportIndexCmd(flags *rootFlags) *cobra.Command {
	var dateFlag, dbFlag string
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Index retained quotation JSON",
		Long: `Load quotation revisions named by index.json into the local store.

This reads retained JSON only. It does not download PDFs. data.db tables are
internal. history and export eod are unchanged. Files that index.json does not
list, wrong contracts, session mismatches, and contradictory nulls are skipped.`,
		Example: `  pse-edge-pp-cli quotation-report index --json
  pse-edge-pp-cli quotation-report index --date 20260924 --json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := parseOptionalSession(dateFlag)
			if err != nil {
				return usageErr(err)
			}
			dataDir, err := cliutil.DataDir()
			if err != nil {
				return configErr(err)
			}
			if dryRunOK(flags) {
				return writeIndexDryRun(cmd.OutOrStdout(), dataDir, session)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			result, err := quotationreport.IndexRetained(ctx, dataDir, quotationDBPath(dbFlag), session)
			if err != nil {
				return err
			}
			return writeQuotationIndex(cmd.OutOrStdout(), flags, result)
		},
	}
	cmd.Flags().StringVar(&dateFlag, "date", "", "One session, YYYYMMDD or YYYY-MM-DD; default is every retained session")
	cmd.Flags().StringVar(&dbFlag, "db", "", "SQLite database path (default: data dir data.db)")
	return cmd
}

func quotationDBPath(flag string) string {
	if strings.TrimSpace(flag) != "" {
		return strings.TrimSpace(flag)
	}
	return defaultDBPath("pse-edge-pp-cli")
}

func quotationRange(dateFlag, fromFlag, toFlag string) (string, string, error) {
	dateFlag = strings.TrimSpace(dateFlag)
	fromFlag = strings.TrimSpace(fromFlag)
	toFlag = strings.TrimSpace(toFlag)
	if dateFlag != "" && (fromFlag != "" || toFlag != "") {
		return "", "", fmt.Errorf("use --date or both --from and --to, not both")
	}
	if dateFlag != "" {
		when, err := parseSessionDate(dateFlag)
		if err != nil {
			return "", "", err
		}
		day := when.Format("2006-01-02")
		return day, day, nil
	}
	if fromFlag == "" || toFlag == "" {
		return "", "", fmt.Errorf("pass --date or both --from and --to")
	}
	from, err := parseSessionDate(fromFlag)
	if err != nil {
		return "", "", fmt.Errorf("invalid --from %q: expected YYYYMMDD or YYYY-MM-DD", fromFlag)
	}
	to, err := parseSessionDate(toFlag)
	if err != nil {
		return "", "", fmt.Errorf("invalid --to %q: expected YYYYMMDD or YYYY-MM-DD", toFlag)
	}
	if to.Before(from) {
		return "", "", fmt.Errorf("--to is before --from")
	}
	return from.Format("2006-01-02"), to.Format("2006-01-02"), nil
}

func parseOptionalSession(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	when, err := parseSessionDate(raw)
	if err != nil {
		return "", err
	}
	return when.Format("2006-01-02"), nil
}

type quotationRowsPayload struct {
	Contract string                    `json:"contract"`
	Status   string                    `json:"status"`
	Selected quotationSelected         `json:"selected"`
	Sessions []quotationSessionPayload `json:"sessions"`
	Limits   quotationRowLimits        `json:"limits"`
}

type quotationSelected struct {
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

type quotationSessionPayload struct {
	SessionDate string                            `json:"session_date"`
	Revision    quotationRevisionPayload          `json:"revision"`
	Rows        []quotationreport.Row             `json:"rows"`
	Ambiguous   []quotationreport.AmbiguousSymbol `json:"ambiguous"`
	Coverage    quotationreport.Coverage          `json:"coverage"`
}

type quotationRevisionPayload struct {
	SHA256     string             `json:"sha256"`
	Current    bool               `json:"current"`
	SourceURL  string             `json:"source_url"`
	AcquiredAt string             `json:"acquired_at"`
	Discovery  quotationDiscovery `json:"discovery"`
}

type quotationDiscovery struct {
	ListingURL          string `json:"listing_url"`
	MatchedTitle        string `json:"matched_title"`
	MatchedCategory     string `json:"matched_category"`
	UploadDate          string `json:"upload_date"`
	DocumentSessionDate string `json:"document_session_date"`
	PDFURL              string `json:"pdf_url"`
}

type quotationRowLimits struct {
	SQLiteInternal string `json:"sqlite_internal"`
	DistinctFrom   string `json:"distinct_from"`
	DashMeaning    string `json:"dash_meaning"`
	Coverage       string `json:"coverage"`
	Revision       string `json:"revision"`
}

type quotationIndexPayload struct {
	Contract string                      `json:"contract"`
	Indexed  []quotationreport.IndexHit  `json:"indexed"`
	Skipped  []quotationreport.IndexSkip `json:"skipped"`
	Limits   quotationIndexLimits        `json:"limits"`
}

type quotationIndexLimits struct {
	SQLiteInternal string `json:"sqlite_internal"`
	NoRefetch      string `json:"no_refetch"`
	NotPromoted    string `json:"not_promoted"`
}

func quotationRowLimitText() quotationRowLimits {
	return quotationRowLimits{
		SQLiteInternal: "data.db tables are internal. This contract is the supported downstream interface.",
		DistinctFrom:   "pse-edge-export-eod-v1 chart history is a different source and is not written by this query.",
		DashMeaning:    "a dash is null reported_dash. It is not encoded as zero.",
		Coverage:       "only admitted local revisions are returned. A missing session is omitted. It is not a zero close.",
		Revision:       "selected.mode current returns is_current revisions. selected.mode sha returns that complete SHA, including a retained previous revision.",
	}
}

func quotationIndexLimitText() quotationIndexLimits {
	return quotationIndexLimits{
		SQLiteInternal: "data.db tables are internal. This contract is the supported downstream interface.",
		NoRefetch:      "index reads retained JSON only. It does not download PDFs.",
		NotPromoted:    "files that index.json does not list as revisions, wrong contracts, session mismatches, and contradictory nulls are skipped.",
	}
}

func loadQuotationRows(ctx context.Context, dbPath, from, to, sha string, symbols []string) (quotationRowsPayload, error) {
	mode := "current"
	if sha != "" {
		mode = "sha"
	}
	payload := quotationRowsPayload{
		Contract: quotationRowsContract,
		Status:   "ok",
		Selected: quotationSelected{Mode: mode, SHA256: sha},
		Sessions: []quotationSessionPayload{},
		Limits:   quotationRowLimitText(),
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			if sha != "" {
				return payload, fmt.Errorf("%w: %s", store.ErrQuotationRevisionNotFound, sha)
			}
			return payload, nil
		}
		return payload, err
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return payload, err
	}
	defer db.Close()
	views, err := db.QueryQuotationRevisions(ctx, store.QuotationQuery{From: from, To: to, SHA: sha})
	if err != nil {
		return payload, err
	}
	for _, view := range views {
		payload.Sessions = append(payload.Sessions, sessionPayload(view, symbols))
	}
	return payload, nil
}

func sessionPayload(view store.QuotationRevisionView, symbols []string) quotationSessionPayload {
	rows := make([]quotationreport.Row, 0, len(view.Rows))
	for _, row := range view.Rows {
		rows = append(rows, storedRowToReport(view, row))
	}
	amb := make([]quotationreport.AmbiguousSymbol, 0, len(view.Ambiguous))
	for _, item := range view.Ambiguous {
		locs := item.Locators
		if locs == nil {
			locs = []string{}
		}
		amb = append(amb, quotationreport.AmbiguousSymbol{Symbol: item.Symbol, Locators: locs})
	}
	parsed := quotationreport.Parsed{SessionDate: view.SessionDate, Rows: rows, Ambiguous: amb}
	cov, filtered, _ := quotationreport.Cover(parsed, symbols)
	if filtered == nil {
		filtered = []quotationreport.Row{}
	}
	cov.Requested = stringSlice(cov.Requested)
	cov.Present = stringSlice(cov.Present)
	cov.Absent = stringSlice(cov.Absent)
	cov.PresentNullClose = stringSlice(cov.PresentNullClose)
	cov.Ambiguous = stringSlice(cov.Ambiguous)
	return quotationSessionPayload{
		SessionDate: view.SessionDate,
		Revision: quotationRevisionPayload{
			SHA256:     view.SourceSHA256,
			Current:    view.Current,
			SourceURL:  view.SourceURL,
			AcquiredAt: view.AcquiredAt,
			Discovery: quotationDiscovery{
				ListingURL:          view.ListingURL,
				MatchedTitle:        view.MatchedTitle,
				MatchedCategory:     view.MatchedCategory,
				UploadDate:          view.UploadDate,
				DocumentSessionDate: view.DocumentSessionDate,
				PDFURL:              view.SourceURL,
			},
		},
		Rows:      filtered,
		Ambiguous: amb,
		Coverage:  cov,
	}
}

func storedRowToReport(view store.QuotationRevisionView, row store.QuotationStoredRow) quotationreport.Row {
	status := row.FieldStatus
	if status == nil {
		status = map[string]string{}
	}
	return quotationreport.Row{
		Symbol:        row.Symbol,
		IssueName:     row.IssueName,
		Board:         row.Board,
		SecurityClass: row.SecurityClass,
		Currency:      row.Currency,
		SessionDate:   view.SessionDate,
		SourceType:    quotationreport.SourceType,
		SourceSHA256:  view.SourceSHA256,
		Page:          row.Page,
		RowLocator:    row.RowLocator,
		Bid:           row.Bid,
		Ask:           row.Ask,
		Open:          row.Open,
		High:          row.High,
		Low:           row.Low,
		Close:         row.Close,
		Volume:        row.Volume,
		Value:         row.Value,
		NetForeign:    row.NetForeign,
		FieldStatus:   status,
	}
}

func stringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func writeQuotationRows(w io.Writer, flags *rootFlags, payload quotationRowsPayload) error {
	if flags != nil && (flags.asJSON || flags.agent) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	if len(payload.Sessions) == 0 {
		fmt.Fprintln(w, "sessions 0")
	}
	for _, session := range payload.Sessions {
		fmt.Fprintf(w, "session %s sha256 %s current %t rows %d\n", session.SessionDate, session.Revision.SHA256, session.Revision.Current, len(session.Rows))
	}
	fmt.Fprintln(w, "use --json for pse-edge-quotation-rows-v1; it is the supported interface")
	return nil
}

func writeIndexDryRun(w io.Writer, root, session string) error {
	sessions, err := quotationreport.SessionDirs(root, session)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(w, "dry-run quotation-report index sessions=0")
		return nil
	}
	for _, day := range sessions {
		fmt.Fprintf(w, "dry-run quotation-report index session=%s\n", day)
	}
	return nil
}

func writeQuotationIndex(w io.Writer, flags *rootFlags, result quotationreport.IndexResult) error {
	if result.Indexed == nil {
		result.Indexed = []quotationreport.IndexHit{}
	}
	if result.Skipped == nil {
		result.Skipped = []quotationreport.IndexSkip{}
	}
	payload := quotationIndexPayload{
		Contract: quotationIndexContract,
		Indexed:  result.Indexed,
		Skipped:  result.Skipped,
		Limits:   quotationIndexLimitText(),
	}
	if flags != nil && (flags.asJSON || flags.agent) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	if len(payload.Indexed) == 0 {
		fmt.Fprintln(w, "indexed 0")
	}
	for _, hit := range payload.Indexed {
		fmt.Fprintf(w, "session %s sha256 %s rows %d\n", hit.SessionDate, hit.SHA256, hit.Rows)
	}
	for _, skip := range payload.Skipped {
		fmt.Fprintf(w, "skipped %s %s\n", skip.Path, skip.Reason)
	}
	fmt.Fprintln(w, "use --json for pse-edge-quotation-index-v1; it is the supported interface")
	return nil
}
