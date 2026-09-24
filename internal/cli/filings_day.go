// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: one Manila calendar day's disclosure bodies and attachments,
// retained as original bytes plus a versioned manifest. Search discovery is
// not a claim that the official corpus is complete.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/pseedge"
)

const disclosureDayContract = "pse-edge-disclosure-day-v1"

const disclosureDayCorpusWarning = "announcements/search.ax is not an authoritative complete corpus. corpus_complete is always false. An empty search or an exhausted page scan does not prove the official viewer has no further filings for this date."

type daySource interface {
	Search(ctx context.Context, search pseedge.DisclosureSearch, page int) (*pseedge.DisclosurePage, error)
	Viewer(ctx context.Context, edgeNo string) (*pseedge.DisclosureViewer, error)
	Document(ctx context.Context, fileID string) ([]byte, string, error)
}

type dayQuery struct {
	Symbol    string `json:"symbol,omitempty"`
	CompanyID string `json:"company_id,omitempty"`
}

type dayDiscovery struct {
	Source         string     `json:"source"`
	Timezone       string     `json:"timezone"`
	FromDate       string     `json:"from_date"`
	ToDate         string     `json:"to_date"`
	Queries        []dayQuery `json:"queries"`
	ScannedPages   int        `json:"scanned_pages"`
	ReportedPages  int        `json:"reported_pages"`
	ReportedCount  int        `json:"reported_count"`
	PageCap        int        `json:"page_cap"`
	PageCapHit     bool       `json:"page_cap_hit"`
	Truncated      bool       `json:"truncated"`
	SearchComplete bool       `json:"search_complete"`
	CorpusComplete bool       `json:"corpus_complete"`
	Failures       []string   `json:"failures"`
	Warnings       []string   `json:"warnings"`
}

type dayArtifact struct {
	Role             string `json:"role"`
	FileID           string `json:"file_id"`
	Label            string `json:"label,omitempty"`
	SourceURL        string `json:"source_url"`
	Outcome          string `json:"outcome"`
	ContentType      string `json:"content_type,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
	RelativePath     string `json:"relative_path,omitempty"`
	Bytes            int    `json:"bytes,omitempty"`
	AcquiredAt       string `json:"acquired_at"`
	Attempts         int    `json:"attempts,omitempty"`
	Error            string `json:"error,omitempty"`
	TextOutcome      string `json:"text_outcome,omitempty"`
	TextRelativePath string `json:"text_relative_path,omitempty"`
	TextError        string `json:"text_error,omitempty"`
}

type dayFiling struct {
	EdgeNo                string        `json:"edge_no"`
	CmpyID                int           `json:"cmpy_id"`
	Symbol                string        `json:"symbol,omitempty"`
	Company               string        `json:"company"`
	Template              string        `json:"template"`
	Title                 string        `json:"title"`
	DisclosedAt           string        `json:"disclosed_at,omitempty"`
	RawTimestamp          string        `json:"raw_timestamp,omitempty"`
	PublicationDateStatus string        `json:"publication_date_status"`
	ViewerURL             string        `json:"viewer_url"`
	ViewerStatus          string        `json:"viewer_status,omitempty"`
	ViewerError           string        `json:"viewer_error,omitempty"`
	InLatestDiscovery     bool          `json:"in_latest_discovery"`
	Artifacts             []dayArtifact `json:"artifacts"`
}

type daySummary struct {
	Filings         int  `json:"filings"`
	ArtifactsOK     int  `json:"artifacts_ok"`
	ArtifactsFailed int  `json:"artifacts_failed"`
	TextFailures    int  `json:"text_failures"`
	Partial         bool `json:"partial"`
	SearchComplete  bool `json:"search_complete"`
	CorpusComplete  bool `json:"corpus_complete"`
	SameDayPartial  bool `json:"same_day_partial"`
}

type dayManifest struct {
	Contract          string       `json:"contract"`
	TargetDate        string       `json:"target_date"`
	Timezone          string       `json:"timezone"`
	AcquisitionCutoff string       `json:"acquisition_cutoff"`
	SameDayPartial    bool         `json:"same_day_partial"`
	OutputDir         string       `json:"output_dir"`
	Discovery         dayDiscovery `json:"discovery"`
	Filings           []dayFiling  `json:"filings"`
	Summary           daySummary   `json:"summary"`
}

type dayCollectOptions struct {
	Target       time.Time
	Now          time.Time
	Queries      []dayQuery
	OutDir       string
	MaxScanPages int
	MaxAttempts  int
	ExtractText  func(ctx context.Context, contentType string, raw []byte) (string, error)
}

var (
	errDayPartial   = errors.New("disclosure day bundle is partial")
	errDayDiscovery = errors.New("disclosure day discovery failed")
)

func newFilingsDayCmd(flags *rootFlags) *cobra.Command {
	var dateFlag string
	var symbols []string
	var outDir string
	var maxScanPages int
	var maxAttempts int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "day",
		Short: "Download one Manila day's disclosure bodies and attachments into a local evidence bundle",
		Long: `Download disclosure publication-date matches for one Asia/Manila calendar
day, including each filing body and every viewer attachment.

--date defaults to today in Asia/Manila. Accepted forms are YYYYMMDD and
MM-DD-YYYY. Selection uses the disclosure publication timestamp from
announcements/search.ax, not the time this command acquired the bytes.
The manifest records acquisition_cutoff. When --date is that same Manila
calendar day, same_day_partial is true: a midday snapshot is not full-day
coverage.

Discovery source is announcements/search.ax (one search per --symbol, or
one market-wide search when --symbol is omitted). corpus_complete is always
false. An empty search, a page cap, or a failed page does not prove the
official viewer has no other filings. Raise --max-scan-pages (default 40,
50 rows per page) when page_cap_hit is true.

Original bytes are kept. A response larger than 32 MiB is a download failure; the prefix is not stored as the document. Extracted text is a companion file. Unavailable
attachments, download failures, rejected error pages, and text-extraction
failures are separate artifact outcomes. Amendments stay distinct edge_no
records. Each rerun fetches retained files again. An identical hash adds
no new file. Changed bytes are stored as a new revision and the previous
file stays. A file with no retained bytes retries until --max-attempts
(default 3).

Exit 0: discovery finished without page-cap or download/viewer failure
(text-extraction failures still exit 0; read summary.text_failures).
Exit 2: usage. Exit 5: discovery failed before any page was scanned.
Exit 6: partial bundle (truncation, viewer failure, or document failure).
The manifest is written in every non-usage case that could create --out.

This command does not publish to a vault and does not replace price exports.`,
		Example: `  pse-edge-pp-cli filings day --date 20260924 --out ./disclosures-20260924 --json
  pse-edge-pp-cli filings day --symbol AT --symbol GTCAP --out ./today --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErr(fmt.Errorf("filings day takes flags, not positional args"))
			}
			if dryRunOK(flags) {
				return nil
			}
			now := time.Now().In(psecal.Manila())
			target, err := parseDisclosureDay(dateFlag, now)
			if err != nil {
				return usageErr(err)
			}
			if outDir == "" {
				return usageErr(fmt.Errorf("--out is required"))
			}
			if maxScanPages <= 0 {
				return usageErr(fmt.Errorf("invalid --max-scan-pages %d: must be positive", maxScanPages))
			}
			if maxAttempts <= 0 {
				return usageErr(fmt.Errorf("invalid --max-attempts %d: must be positive", maxAttempts))
			}
			queries := make([]dayQuery, 0, len(symbols))
			if len(symbols) == 0 {
				queries = append(queries, dayQuery{})
			} else {
				if dbPath == "" {
					dbPath = defaultDBPath("pse-edge-pp-cli")
				}
				seen := map[string]struct{}{}
				for _, sym := range symbols {
					rc, err := resolvePSECompany(cmd.Context(), cmd, flags, dbPath, sym)
					if err != nil {
						return err
					}
					id := fmt.Sprintf("%d", rc.CmpyID)
					if _, ok := seen[id]; ok {
						continue
					}
					seen[id] = struct{}{}
					queries = append(queries, dayQuery{Symbol: rc.Symbol, CompanyID: id})
				}
			}
			hc := httpClient60()
			src := liveDaySource{hc: hc}
			manifest, err := collectDisclosureDay(cmd.Context(), src, dayCollectOptions{
				Target:       target,
				Now:          now,
				Queries:      queries,
				OutDir:       outDir,
				MaxScanPages: maxScanPages,
				MaxAttempts:  maxAttempts,
				ExtractText:  extractDisclosureDayText,
			})
			if printErr := printJSONFiltered(cmd.OutOrStdout(), manifest, flags); printErr != nil {
				return printErr
			}
			if errors.Is(err, errDayDiscovery) {
				return apiErr(err)
			}
			if errors.Is(err, errDayPartial) {
				return partialFailureErr(err)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&dateFlag, "date", "", "Publication date in Asia/Manila, YYYYMMDD or MM-DD-YYYY (default: today)")
	cmd.Flags().StringArrayVar(&symbols, "symbol", nil, "Limit discovery to this symbol (repeatable). Omit for a market-wide search")
	cmd.Flags().StringVar(&outDir, "out", "", "Directory for manifest.json and retained files (required)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 40, "Maximum search pages per query (50 rows/page). A hit is reported; it is not silent truncation")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 3, "Bounded download attempts per file across reruns")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path used to resolve --symbol (default: resolved data directory data.db)")
	return cmd
}

func httpClient60() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

type liveDaySource struct {
	hc *http.Client
}

func (s liveDaySource) Search(ctx context.Context, search pseedge.DisclosureSearch, page int) (*pseedge.DisclosurePage, error) {
	return pseedge.FetchDisclosurePage(ctx, s.hc, search, page)
}

func (s liveDaySource) Viewer(ctx context.Context, edgeNo string) (*pseedge.DisclosureViewer, error) {
	return pseedge.FetchDisclosureViewer(ctx, s.hc, edgeNo)
}

func (s liveDaySource) Document(ctx context.Context, fileID string) ([]byte, string, error) {
	return pseedge.FetchDisclosureDocument(ctx, s.hc, fileID)
}

func parseDisclosureDay(raw string, now time.Time) (time.Time, error) {
	now = now.In(psecal.Manila())
	raw = strings.TrimSpace(raw)
	var parsed time.Time
	var err error
	if raw == "" {
		parsed = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, psecal.Manila())
	} else if len(raw) == 8 && !strings.Contains(raw, "-") {
		parsed, err = time.ParseInLocation("20060102", raw, psecal.Manila())
	} else {
		parsed, err = time.ParseInLocation("01-02-2006", raw, psecal.Manila())
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --date %q: expected YYYYMMDD or MM-DD-YYYY", raw)
	}
	parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, psecal.Manila())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, psecal.Manila())
	if parsed.After(today) {
		return time.Time{}, fmt.Errorf("invalid --date %s: after today in Asia/Manila", parsed.Format("20060102"))
	}
	return parsed, nil
}

func extractDisclosureDayText(ctx context.Context, contentType string, raw []byte) (string, error) {
	if contentType == "application/pdf" || looksLikePDF(raw) {
		return extractDisclosurePDFText(ctx, raw)
	}
	text := visibleHTMLBodyText(raw)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no extractable text")
	}
	return text, nil
}

func collectDisclosureDay(ctx context.Context, src daySource, opt dayCollectOptions) (dayManifest, error) {
	if opt.MaxScanPages <= 0 {
		opt.MaxScanPages = 40
	}
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = 3
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	now := opt.Now.In(psecal.Manila())
	target := opt.Target.In(psecal.Manila())
	absOut, err := filepath.Abs(opt.OutDir)
	if err != nil {
		return dayManifest{}, usageErr(fmt.Errorf("invalid --out: %w", err))
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return dayManifest{}, fmt.Errorf("creating --out: %w", err)
	}
	prior, err := loadDayManifest(absOut)
	if err != nil {
		return dayManifest{}, err
	}
	targetKey := target.Format("20060102")
	if prior.Contract != "" && (prior.Contract != disclosureDayContract || prior.TargetDate != targetKey) {
		return dayManifest{}, usageErr(fmt.Errorf("--out already holds %s for %s", prior.Contract, prior.TargetDate))
	}

	mdDate := target.Format("01-02-2006")
	discovery := dayDiscovery{
		Source:         "announcements/search.ax",
		Timezone:       "Asia/Manila",
		FromDate:       mdDate,
		ToDate:         mdDate,
		Queries:        opt.Queries,
		PageCap:        opt.MaxScanPages,
		CorpusComplete: false,
		Warnings:       []string{disclosureDayCorpusWarning},
		Failures:       []string{},
	}
	if len(discovery.Queries) == 0 {
		discovery.Queries = []dayQuery{{}}
	}

	var found []pseedge.Disclosure
	queriesComplete := true
	pagesScanned := 0
	for _, q := range discovery.Queries {
		search := pseedge.DisclosureSearch{CompanyID: q.CompanyID, FromDate: mdDate, ToDate: mdDate}
		queryPages := 0
		for pageNo := 1; ; pageNo++ {
			if queryPages >= opt.MaxScanPages {
				queriesComplete = false
				discovery.PageCapHit = true
				discovery.Truncated = true
				break
			}
			page, err := src.Search(ctx, search, pageNo)
			if err != nil {
				discovery.Failures = append(discovery.Failures, fmt.Sprintf("company %q page %d: %s", q.CompanyID, pageNo, err.Error()))
				queriesComplete = false
				break
			}
			queryPages++
			pagesScanned++
			if pageNo == 1 {
				discovery.ReportedPages += page.TotalPages
				discovery.ReportedCount += page.TotalCount
			}
			for _, row := range page.Rows {
				if q.CompanyID != "" && fmt.Sprintf("%d", row.CmpyID) != q.CompanyID {
					continue
				}
				found = append(found, row)
			}
			if page.TotalPages <= pageNo {
				break
			}
		}
		if queryPages < 1 {
			queriesComplete = false
		}
	}
	discovery.ScannedPages = pagesScanned
	discovery.SearchComplete = queriesComplete && !discovery.PageCapHit && len(discovery.Failures) == 0
	if pagesScanned == 0 {
		discovery.SearchComplete = false
	}

	symbolByCompany := map[int]string{}
	for _, q := range discovery.Queries {
		if q.CompanyID == "" || q.Symbol == "" {
			continue
		}
		var id int
		fmt.Sscanf(q.CompanyID, "%d", &id)
		symbolByCompany[id] = q.Symbol
	}
	kept := filterDisclosureDayRows(found, target)
	manifest := prior
	manifest.Contract = disclosureDayContract
	manifest.TargetDate = targetKey
	manifest.Timezone = "Asia/Manila"
	manifest.AcquisitionCutoff = now.Format(time.RFC3339)
	manifest.SameDayPartial = targetKey == now.Format("20060102")
	manifest.OutputDir = absOut
	manifest.Discovery = discovery
	if manifest.Filings == nil {
		manifest.Filings = []dayFiling{}
	}
	for i := range manifest.Filings {
		manifest.Filings[i].InLatestDiscovery = false
	}
	manifest.Filings = mergeDayFilings(manifest.Filings, kept, symbolByCompany)

	partial := !discovery.SearchComplete
	if pagesScanned == 0 {
		manifest = summarizeDay(manifest)
		if err := saveDayManifest(absOut, manifest); err != nil {
			return manifest, err
		}
		return manifest, fmt.Errorf("%w: %s", errDayDiscovery, strings.Join(discovery.Failures, "; "))
	}

	latest := map[string]struct{}{}
	for _, row := range kept {
		latest[row.EdgeNo] = struct{}{}
	}
	for i := range manifest.Filings {
		filing := &manifest.Filings[i]
		if _, ok := latest[filing.EdgeNo]; !ok {
			continue
		}
		filing.InLatestDiscovery = true
		viewer, err := src.Viewer(ctx, filing.EdgeNo)
		if err != nil {
			filing.ViewerStatus = "failed"
			filing.ViewerError = err.Error()
			partial = true
			continue
		}
		filing.ViewerStatus = "ok"
		filing.ViewerError = ""
		if filing.ViewerURL == "" {
			filing.ViewerURL = viewer.ViewerURL
		}
		files := dayFileList(viewer)
		for _, item := range files {
			art, failed := retainDayFile(ctx, src, opt, absOut, filing, item, now)
			if art != nil {
				filing.Artifacts = append(filing.Artifacts, *art)
			}
			if failed {
				partial = true
			}
		}
	}
	manifest = summarizeDay(manifest)
	if err := saveDayManifest(absOut, manifest); err != nil {
		return manifest, err
	}
	if partial || manifest.Summary.Partial {
		return manifest, errDayPartial
	}
	return manifest, nil
}

type dayFileItem struct {
	FileID string
	Role   string
	Label  string
}

func dayFileList(viewer *pseedge.DisclosureViewer) []dayFileItem {
	seen := map[string]struct{}{}
	var out []dayFileItem
	if id := strings.TrimSpace(viewer.DocumentFileID); id != "" {
		seen[id] = struct{}{}
		out = append(out, dayFileItem{FileID: id, Role: "body"})
	}
	for _, att := range viewer.Attachments {
		id := strings.TrimSpace(att.FileID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, dayFileItem{FileID: id, Role: "attachment", Label: att.Label})
	}
	return out
}

func filterDisclosureDayRows(rows []pseedge.Disclosure, target time.Time) []pseedge.Disclosure {
	want := target.In(psecal.Manila()).Format("2006-01-02")
	seen := map[string]struct{}{}
	out := make([]pseedge.Disclosure, 0, len(rows))
	for _, row := range rows {
		if row.EdgeNo == "" {
			continue
		}
		if _, ok := seen[row.EdgeNo]; ok {
			continue
		}
		if row.DisclosedAt != "" {
			if len(row.DisclosedAt) < 10 || row.DisclosedAt[:10] != want {
				continue
			}
		}
		seen[row.EdgeNo] = struct{}{}
		out = append(out, row)
	}
	return out
}

func mergeDayFilings(prior []dayFiling, rows []pseedge.Disclosure, symbols map[int]string) []dayFiling {
	index := map[string]int{}
	out := append([]dayFiling{}, prior...)
	for i := range out {
		index[out[i].EdgeNo] = i
	}
	for _, row := range rows {
		status := "matched"
		if row.DisclosedAt == "" {
			status = "unparsed_kept"
		}
		sym := symbols[row.CmpyID]
		if i, ok := index[row.EdgeNo]; ok {
			out[i].CmpyID = row.CmpyID
			out[i].Company = row.Company
			out[i].Template = row.Template
			out[i].Title = row.Title
			out[i].DisclosedAt = row.DisclosedAt
			out[i].RawTimestamp = row.RawTimestamp
			out[i].PublicationDateStatus = status
			if sym != "" {
				out[i].Symbol = sym
			}
			if out[i].ViewerURL == "" {
				out[i].ViewerURL = pseedge.ViewerURL(row.EdgeNo)
			}
			continue
		}
		out = append(out, dayFiling{
			EdgeNo:                row.EdgeNo,
			CmpyID:                row.CmpyID,
			Symbol:                sym,
			Company:               row.Company,
			Template:              row.Template,
			Title:                 row.Title,
			DisclosedAt:           row.DisclosedAt,
			RawTimestamp:          row.RawTimestamp,
			PublicationDateStatus: status,
			ViewerURL:             pseedge.ViewerURL(row.EdgeNo),
			Artifacts:             []dayArtifact{},
		})
		index[row.EdgeNo] = len(out) - 1
	}
	return out
}

func retainDayFile(ctx context.Context, src daySource, opt dayCollectOptions, outDir string, filing *dayFiling, item dayFileItem, now time.Time) (*dayArtifact, bool) {
	if !pseedge.ValidDocumentFileID(item.FileID) || !validEdgeNo(filing.EdgeNo) {
		return &dayArtifact{
			Role: item.Role, FileID: item.FileID, Label: item.Label,
			SourceURL: pseedge.DocumentURL(item.FileID), Outcome: "download_failed",
			AcquiredAt: now.Format(time.RFC3339), Error: "unsafe file_id or edge_no",
		}, true
	}
	hasBytes := matchingRetainedArtifact(filing.Artifacts, item.FileID, outDir) != nil
	attempts := failureAttempts(filing.Artifacts, item.FileID)
	if !hasBytes && attempts >= opt.MaxAttempts {
		return nil, true
	}
	body, headerCT, err := src.Document(ctx, item.FileID)
	acquired := now.Format(time.RFC3339)
	base := dayArtifact{
		Role: item.Role, FileID: item.FileID, Label: item.Label,
		SourceURL: pseedge.DocumentURL(item.FileID), AcquiredAt: acquired,
		Attempts: attempts + 1,
	}
	if err != nil {
		var payload *pseedge.DisclosurePayloadError
		if errors.As(err, &payload) {
			base.Outcome = payload.Kind
			base.Error = payload.Reason
		} else {
			base.Outcome = "download_failed"
			base.Error = err.Error()
		}
		return &base, true
	}
	contentType, classErr := pseedge.ClassifyDisclosurePayload(body, headerCT)
	if classErr != nil {
		var payload *pseedge.DisclosurePayloadError
		if errors.As(classErr, &payload) {
			base.Outcome = payload.Kind
			base.Error = payload.Reason
		} else {
			base.Outcome = "rejected_error_page"
			base.Error = classErr.Error()
		}
		return &base, true
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	if existing := artifactWithHash(filing.Artifacts, item.FileID, digest, outDir); existing != nil {
		return nil, false
	}
	name := item.FileID + ".bin"
	outcome := "downloaded"
	if hasOtherHash(filing.Artifacts, item.FileID, digest) || fileExistsDifferent(outDir, filing.EdgeNo, name, digest) {
		name = item.FileID + "." + digest[:12] + ".bin"
		outcome = "revised"
	}
	rel := filepath.ToSlash(filepath.Join("files", filing.EdgeNo, name))
	dest, err := containedJoin(outDir, "files", filing.EdgeNo, name)
	if err != nil {
		base.Outcome = "download_failed"
		base.Error = err.Error()
		return &base, true
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		base.Outcome = "download_failed"
		base.Error = err.Error()
		return &base, true
	}
	if _, err := os.Stat(dest); err == nil {
		base.Outcome = "download_failed"
		base.Error = "refusing to overwrite " + rel
		return &base, true
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		base.Outcome = "download_failed"
		base.Error = err.Error()
		return &base, true
	}
	base.Outcome = outcome
	base.ContentType = contentType
	base.SHA256 = digest
	base.RelativePath = rel
	base.Bytes = len(body)
	if opt.ExtractText != nil {
		text, textErr := opt.ExtractText(ctx, contentType, body)
		if textErr != nil {
			base.TextOutcome = "failed"
			base.TextError = textErr.Error()
		} else {
			textName := strings.TrimSuffix(name, ".bin") + ".txt"
			textRel := filepath.ToSlash(filepath.Join("files", filing.EdgeNo, textName))
			textDest, destErr := containedJoin(outDir, "files", filing.EdgeNo, textName)
			if destErr != nil {
				base.TextOutcome = "failed"
				base.TextError = destErr.Error()
			} else if _, statErr := os.Stat(textDest); statErr == nil {
				base.TextOutcome = "extracted"
				base.TextRelativePath = textRel
			} else if writeErr := os.WriteFile(textDest, []byte(text), 0o644); writeErr != nil {
				base.TextOutcome = "failed"
				base.TextError = writeErr.Error()
			} else {
				base.TextOutcome = "extracted"
				base.TextRelativePath = textRel
			}
		}
	}
	return &base, false
}

func matchingRetainedArtifact(arts []dayArtifact, fileID, outDir string) *dayArtifact {
	for i := range arts {
		art := &arts[i]
		if art.FileID != fileID || art.SHA256 == "" || art.RelativePath == "" {
			continue
		}
		if art.Outcome != "downloaded" && art.Outcome != "revised" && art.Outcome != "unchanged" {
			continue
		}
		if fileHashMatches(outDir, art.RelativePath, art.SHA256) {
			return art
		}
	}
	return nil
}

func artifactWithHash(arts []dayArtifact, fileID, digest, outDir string) *dayArtifact {
	for i := range arts {
		art := &arts[i]
		if art.FileID == fileID && art.SHA256 == digest && art.RelativePath != "" && fileHashMatches(outDir, art.RelativePath, digest) {
			return art
		}
	}
	return nil
}

func hasOtherHash(arts []dayArtifact, fileID, digest string) bool {
	for _, art := range arts {
		if art.FileID == fileID && art.SHA256 != "" && art.SHA256 != digest && (art.Outcome == "downloaded" || art.Outcome == "revised") {
			return true
		}
	}
	return false
}

func failureAttempts(arts []dayArtifact, fileID string) int {
	n := 0
	for _, art := range arts {
		if art.FileID != fileID {
			continue
		}
		if art.Outcome == "downloaded" || art.Outcome == "revised" || art.Outcome == "unchanged" {
			continue
		}
		if art.Attempts > n {
			n = art.Attempts
		}
	}
	return n
}

func fileExistsDifferent(outDir, edgeNo, name, digest string) bool {
	path, err := containedJoin(outDir, "files", edgeNo, name)
	if err != nil {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) != digest
}

func fileHashMatches(outDir, rel, digest string) bool {
	if rel == "" || strings.Contains(rel, "..") {
		return false
	}
	parts := strings.Split(filepath.FromSlash(rel), string(os.PathSeparator))
	path, err := containedJoin(outDir, parts...)
	if err != nil {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) == digest
}

func validEdgeNo(edgeNo string) bool {
	if len(edgeNo) < 16 || len(edgeNo) > 128 {
		return false
	}
	for _, r := range edgeNo {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !ok {
			return false
		}
	}
	return true
}

func containedJoin(root string, parts ...string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\`) {
			return "", fmt.Errorf("unsafe path segment %q", part)
		}
	}
	joined := filepath.Join(append([]string{absRoot}, parts...)...)
	rel, err := filepath.Rel(absRoot, joined)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes output directory")
	}
	return joined, nil
}

func summarizeDay(manifest dayManifest) dayManifest {
	manifest.Discovery.CorpusComplete = false
	summary := daySummary{
		Filings:        len(manifest.Filings),
		SearchComplete: manifest.Discovery.SearchComplete,
		CorpusComplete: false,
		SameDayPartial: manifest.SameDayPartial,
	}
	for _, filing := range manifest.Filings {
		if filing.ViewerStatus == "failed" {
			summary.Partial = true
		}
		latest := map[string]dayArtifact{}
		for _, art := range filing.Artifacts {
			latest[art.FileID] = art
		}
		for _, art := range latest {
			switch art.Outcome {
			case "downloaded", "revised", "unchanged":
				if art.SHA256 != "" && art.RelativePath != "" {
					summary.ArtifactsOK++
				}
				if art.TextOutcome == "failed" {
					summary.TextFailures++
				}
			default:
				summary.ArtifactsFailed++
				summary.Partial = true
			}
		}
	}
	if !manifest.Discovery.SearchComplete {
		summary.Partial = true
	}
	manifest.Summary = summary
	return manifest
}

func loadDayManifest(outDir string) (dayManifest, error) {
	path, err := containedJoin(outDir, "manifest.json")
	if err != nil {
		return dayManifest{}, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return dayManifest{Filings: []dayFiling{}}, nil
	}
	if err != nil {
		return dayManifest{}, err
	}
	var manifest dayManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return dayManifest{}, fmt.Errorf("reading manifest.json: %w", err)
	}
	if manifest.Filings == nil {
		manifest.Filings = []dayFiling{}
	}
	return manifest, nil
}

func saveDayManifest(outDir string, manifest dayManifest) error {
	path, err := containedJoin(outDir, "manifest.json")
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
