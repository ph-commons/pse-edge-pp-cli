// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
	"github.com/ph-commons/pse-edge-pp-cli/internal/pseedge"
)

const (
	dayEdgeA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dayEdgeB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	dayEdgeC = "cccccccccccccccccccccccccccccccc"
)

type fakeDoc struct {
	body []byte
	ct   string
	err  error
}

type fakeDaySource struct {
	pages     map[string][]*pseedge.DisclosurePage
	viewers   map[string]*pseedge.DisclosureViewer
	viewerErr map[string]error
	docs      map[string][]fakeDoc
	docCalls  map[string]int
	searchErr map[string]error
}

func (f *fakeDaySource) Search(_ context.Context, search pseedge.DisclosureSearch, page int) (*pseedge.DisclosurePage, error) {
	if err := f.searchErr[search.CompanyID]; err != nil && page == 1 {
		return nil, err
	}
	set := f.pages[search.CompanyID]
	if page < 1 || page > len(set) {
		return &pseedge.DisclosurePage{PageNo: page}, nil
	}
	return set[page-1], nil
}

func (f *fakeDaySource) Viewer(_ context.Context, edgeNo string) (*pseedge.DisclosureViewer, error) {
	if err := f.viewerErr[edgeNo]; err != nil {
		return nil, err
	}
	v := f.viewers[edgeNo]
	if v == nil {
		return nil, errors.New("missing viewer")
	}
	return v, nil
}

func (f *fakeDaySource) Document(_ context.Context, fileID string) ([]byte, string, error) {
	if f.docCalls == nil {
		f.docCalls = map[string]int{}
	}
	n := f.docCalls[fileID]
	f.docCalls[fileID] = n + 1
	seq := f.docs[fileID]
	if len(seq) == 0 {
		return nil, "", errors.New("no document")
	}
	if n >= len(seq) {
		n = len(seq) - 1
	}
	item := seq[n]
	return item.body, item.ct, item.err
}

func manilaDay(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, psecal.Manila())
}

func dayOpts(dir string, now, target time.Time, queries []dayQuery, pages, attempts int) dayCollectOptions {
	return dayCollectOptions{
		Target: target, Now: now, Queries: queries, OutDir: dir,
		MaxScanPages: pages, MaxAttempts: attempts,
		ExtractText: func(_ context.Context, contentType string, raw []byte) (string, error) {
			if contentType == "application/pdf" && strings.Contains(string(raw), "NOTEXT") {
				return "", errors.New("no text layer")
			}
			if strings.Contains(string(raw), "BADTEXT") {
				return "", errors.New("extract failed")
			}
			return "extracted " + string(raw), nil
		},
	}
}

func page(totalPages, totalCount int, rows ...pseedge.Disclosure) *pseedge.DisclosurePage {
	return &pseedge.DisclosurePage{Rows: rows, PageNo: 1, TotalPages: totalPages, TotalCount: totalCount}
}

func row(edge string, cmpy int, at string) pseedge.Disclosure {
	return pseedge.Disclosure{EdgeNo: edge, CmpyID: cmpy, Company: "Co", Template: "17-Q", Title: "Quarterly Report", DisclosedAt: at}
}

func viewer(edge, doc string, attachments ...pseedge.DisclosureAttachment) *pseedge.DisclosureViewer {
	return &pseedge.DisclosureViewer{EdgeNo: edge, DocumentFileID: doc, Attachments: attachments, ViewerURL: pseedge.ViewerURL(edge), Company: "Co", Title: "Quarterly Report"}
}

func TestCollectDisclosureDayBundle(t *testing.T) {
	dir := t.TempDir()
	now := manilaDay(2026, 9, 25, 9)
	target := manilaDay(2026, 9, 24, 0)
	at := "2026-09-24T15:04:00+08:00"
	src := &fakeDaySource{
		pages: map[string][]*pseedge.DisclosurePage{
			"5": {page(1, 2,
				row(dayEdgeA, 5, at),
				row(dayEdgeB, 5, at),
				row(dayEdgeC, 9, at),
				row("dddddddddddddddddddddddddddddddd", 5, "2026-09-23T15:04:00+08:00"),
				pseedge.Disclosure{EdgeNo: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", CmpyID: 5, Company: "Co", Template: "17-C", Title: "Unparsed", RawTimestamp: "not-a-date"},
			)},
		},
		viewers: map[string]*pseedge.DisclosureViewer{
			dayEdgeA:                           viewer(dayEdgeA, "100", pseedge.DisclosureAttachment{FileID: "201", Label: "Annex"}, pseedge.DisclosureAttachment{FileID: "202", Label: "Scan"}),
			dayEdgeB:                           viewer(dayEdgeB, "300"),
			"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee": viewer("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "400"),
		},
		docs: map[string][]fakeDoc{
			"100": {{body: []byte("<html><body>Quarterly notes</body></html>"), ct: "text/html"}},
			"201": {{body: []byte("%PDF-1.4\nannex"), ct: "application/pdf"}},
			"202": {{body: []byte("%PDF-1.4\nNOTEXT"), ct: "application/pdf"}},
			"300": {{body: []byte("<html><title>Error</title>nope</html>"), ct: "text/html"}},
			"400": {{body: []byte("<html><body>kept</body></html>"), ct: "text/html"}},
		},
	}
	manifest, err := collectDisclosureDay(context.Background(), src, dayOpts(dir, now, target, []dayQuery{{Symbol: "AT", CompanyID: "5"}}, 40, 3))
	if !errors.Is(err, errDayPartial) {
		t.Fatalf("err = %v, want partial", err)
	}
	if manifest.Discovery.CorpusComplete || manifest.Summary.CorpusComplete {
		t.Fatal("corpus_complete must stay false")
	}
	if !manifest.Discovery.SearchComplete {
		t.Fatalf("search should be complete: %+v", manifest.Discovery)
	}
	if manifest.SameDayPartial {
		t.Fatal("past date is not a same-day partial snapshot")
	}
	if manifest.AcquisitionCutoff == "" {
		t.Fatal("missing acquisition cutoff")
	}
	edges := map[string]dayFiling{}
	for _, filing := range manifest.Filings {
		edges[filing.EdgeNo] = filing
	}
	if _, ok := edges[dayEdgeC]; ok {
		t.Fatal("other issuer leaked through the symbol filter")
	}
	if _, ok := edges["dddddddddddddddddddddddddddddddd"]; ok {
		t.Fatal("previous publication date was kept")
	}
	unparsed, ok := edges["eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"]
	if !ok || unparsed.PublicationDateStatus != "unparsed_kept" {
		t.Fatalf("unparsed search-window row = %+v", unparsed)
	}
	a := edges[dayEdgeA]
	b := edges[dayEdgeB]
	if a.Symbol != "AT" || b.Template != "17-Q" {
		t.Fatalf("identity A=%+v B=%+v", a, b)
	}
	if len(a.Artifacts) != 3 {
		t.Fatalf("want body plus two attachments, got %+v", a.Artifacts)
	}
	var pdf, html, failedText int
	for _, art := range a.Artifacts {
		if art.SHA256 == "" || art.RelativePath == "" || art.SourceURL == "" {
			t.Fatalf("artifact missing trace: %+v", art)
		}
		body, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(art.RelativePath)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != art.SHA256 {
			t.Fatalf("hash mismatch for %s", art.FileID)
		}
		switch art.ContentType {
		case "application/pdf":
			pdf++
		case "text/html":
			html++
		}
		if art.TextOutcome == "failed" {
			failedText++
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(art.RelativePath))); err != nil {
				t.Fatal("raw bytes dropped after text failure")
			}
		}
	}
	if pdf != 2 || html != 1 || failedText != 1 {
		t.Fatalf("pdf=%d html=%d textFailures=%d", pdf, html, failedText)
	}
	if b.Artifacts[0].Outcome != "rejected_error_page" {
		t.Fatalf("error page outcome = %+v", b.Artifacts[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "files", dayEdgeB, "300.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("error page was stored as a document")
	}
	if !strings.Contains(manifest.Discovery.Warnings[0], "not an authoritative complete corpus") {
		t.Fatalf("warnings = %v", manifest.Discovery.Warnings)
	}
}

func TestCollectDisclosureDayRerunResumeRevision(t *testing.T) {
	dir := t.TempDir()
	now := manilaDay(2026, 9, 24, 12)
	target := manilaDay(2026, 9, 24, 0)
	at := "2026-09-24T09:00:00+08:00"
	src := &fakeDaySource{
		pages: map[string][]*pseedge.DisclosurePage{
			"": {page(1, 1, row(dayEdgeA, 5, at))},
		},
		viewers: map[string]*pseedge.DisclosureViewer{
			dayEdgeA: viewer(dayEdgeA, "100", pseedge.DisclosureAttachment{FileID: "201", Label: "Annex"}),
		},
		docs: map[string][]fakeDoc{
			"100": {{body: []byte("<html><body>v1</body></html>")}, {body: []byte("<html><body>v2</body></html>")}},
			"201": {{err: errors.New("timeout")}, {body: []byte("<html><body>annex</body></html>")}},
		},
	}
	opt := dayOpts(dir, now, target, []dayQuery{{}}, 40, 3)
	first, err := collectDisclosureDay(context.Background(), src, opt)
	if !errors.Is(err, errDayPartial) {
		t.Fatalf("first err = %v", err)
	}
	if !first.SameDayPartial {
		t.Fatal("midday snapshot must set same_day_partial")
	}
	second, err := collectDisclosureDay(context.Background(), src, opt)
	if err != nil {
		t.Fatalf("resume err = %v", err)
	}
	if src.docCalls["100"] != 2 {
		t.Fatalf("body calls = %d, want 2 (revision fetched once more)", src.docCalls["100"])
	}
	if src.docCalls["201"] != 2 {
		t.Fatalf("attachment calls = %d, want retry", src.docCalls["201"])
	}
	filing := second.Filings[0]
	var revised, retried bool
	for _, art := range filing.Artifacts {
		if art.FileID == "100" && art.Outcome == "downloaded" {
			old, readErr := os.ReadFile(filepath.Join(dir, "files", dayEdgeA, "100.bin"))
			if readErr != nil || string(old) != "<html><body>v1</body></html>" {
				t.Fatalf("original bytes overwritten: %q %v", old, readErr)
			}
		}
		if art.FileID == "100" && art.Outcome == "revised" {
			revised = true
			body, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(art.RelativePath)))
			if readErr != nil || !strings.Contains(string(body), "v2") {
				t.Fatal("revision bytes missing")
			}
		}
		if art.FileID == "201" && art.Outcome == "downloaded" {
			retried = true
		}
	}
	if !revised || !retried {
		t.Fatalf("revised=%v retried=%v arts=%+v", revised, retried, filing.Artifacts)
	}
	third, err := collectDisclosureDay(context.Background(), src, opt)
	if err != nil {
		t.Fatalf("unchanged rerun err = %v", err)
	}
	if src.docCalls["100"] != 3 || src.docCalls["201"] != 3 {
		t.Fatalf("unchanged rerun did not re-check bytes: %+v", src.docCalls)
	}
	if len(third.Filings[0].Artifacts) != len(second.Filings[0].Artifacts) {
		t.Fatal("unchanged rerun duplicated artifacts")
	}

	src.pages[""] = []*pseedge.DisclosurePage{page(1, 2, row(dayEdgeA, 5, at), row(dayEdgeB, 5, at))}
	src.viewers[dayEdgeB] = viewer(dayEdgeB, "300")
	src.docs["300"] = []fakeDoc{{body: []byte("<html><body>later</body></html>")}}
	added, err := collectDisclosureDay(context.Background(), src, opt)
	if err != nil {
		t.Fatalf("later addition err = %v", err)
	}
	if len(added.Filings) != 2 || added.Filings[1].EdgeNo != dayEdgeB {
		t.Fatalf("later filing missing: %+v", added.Filings)
	}
}

func TestCollectDisclosureDayPageCapEmptyAndAttemptBound(t *testing.T) {
	dir := t.TempDir()
	now := manilaDay(2026, 9, 25, 9)
	target := manilaDay(2026, 9, 24, 0)
	at := "2026-09-24T15:04:00+08:00"
	src := &fakeDaySource{
		pages: map[string][]*pseedge.DisclosurePage{
			"": {
				{Rows: []pseedge.Disclosure{row(dayEdgeA, 1, at)}, PageNo: 1, TotalPages: 2, TotalCount: 2},
				{Rows: []pseedge.Disclosure{row(dayEdgeB, 1, at)}, PageNo: 2, TotalPages: 2, TotalCount: 2},
			},
		},
		viewers: map[string]*pseedge.DisclosureViewer{dayEdgeA: viewer(dayEdgeA, "100")},
		docs:    map[string][]fakeDoc{"100": {{body: []byte("<html><body>one</body></html>")}}},
	}
	manifest, err := collectDisclosureDay(context.Background(), src, dayOpts(dir, now, target, nil, 1, 3))
	if !errors.Is(err, errDayPartial) || !manifest.Discovery.PageCapHit || manifest.Discovery.SearchComplete {
		t.Fatalf("cap err=%v discovery=%+v", err, manifest.Discovery)
	}
	if len(manifest.Filings) != 1 || manifest.Filings[0].EdgeNo != dayEdgeA {
		t.Fatalf("truncated discovery kept wrong rows: %+v", manifest.Filings)
	}
	if manifest.Discovery.CorpusComplete {
		t.Fatal("page cap must not claim corpus completeness")
	}

	emptyDir := t.TempDir()
	emptySrc := &fakeDaySource{pages: map[string][]*pseedge.DisclosurePage{"": {page(0, 0)}}}
	empty, err := collectDisclosureDay(context.Background(), emptySrc, dayOpts(emptyDir, now, target, nil, 40, 3))
	if err != nil {
		t.Fatalf("empty search err = %v", err)
	}
	if empty.Discovery.CorpusComplete || !empty.Discovery.SearchComplete || len(empty.Filings) != 0 {
		t.Fatalf("empty discovery = %+v filings=%d", empty.Discovery, len(empty.Filings))
	}
	if !strings.Contains(empty.Discovery.Warnings[0], "empty search") && !strings.Contains(empty.Discovery.Warnings[0], "does not prove") {
		t.Fatalf("warnings = %v", empty.Discovery.Warnings)
	}

	failDir := t.TempDir()
	failSrc := &fakeDaySource{searchErr: map[string]error{"": errors.New("upstream down")}}
	failed, err := collectDisclosureDay(context.Background(), failSrc, dayOpts(failDir, now, target, nil, 40, 3))
	if !errors.Is(err, errDayDiscovery) || failed.Discovery.CorpusComplete || failed.Discovery.SearchComplete {
		t.Fatalf("discovery failure err=%v %+v", err, failed.Discovery)
	}

	boundDir := t.TempDir()
	bound := &fakeDaySource{
		pages:   map[string][]*pseedge.DisclosurePage{"": {page(1, 1, row(dayEdgeA, 1, at))}},
		viewers: map[string]*pseedge.DisclosureViewer{dayEdgeA: viewer(dayEdgeA, "100")},
		docs:    map[string][]fakeDoc{"100": {{err: errors.New("down")}}},
	}
	opt := dayOpts(boundDir, now, target, nil, 40, 1)
	if _, err := collectDisclosureDay(context.Background(), bound, opt); !errors.Is(err, errDayPartial) {
		t.Fatal(err)
	}
	if _, err := collectDisclosureDay(context.Background(), bound, opt); !errors.Is(err, errDayPartial) {
		t.Fatal(err)
	}
	if bound.docCalls["100"] != 1 {
		t.Fatalf("attempts not bounded: %d", bound.docCalls["100"])
	}
}

func TestCollectDisclosureDayContainment(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	now := manilaDay(2026, 9, 25, 9)
	target := manilaDay(2026, 9, 24, 0)
	edge := dayEdgeA
	src := &fakeDaySource{
		pages: map[string][]*pseedge.DisclosurePage{"": {page(1, 1, row(edge, 1, "2026-09-24T10:00:00+08:00"))}},
		viewers: map[string]*pseedge.DisclosureViewer{
			edge: viewer(edge, "../evil", pseedge.DisclosureAttachment{FileID: "12/34"}),
		},
	}
	manifest, err := collectDisclosureDay(context.Background(), src, dayOpts(dir, now, target, nil, 40, 3))
	if !errors.Is(err, errDayPartial) {
		t.Fatalf("err = %v", err)
	}
	for _, art := range manifest.Filings[0].Artifacts {
		if art.RelativePath != "" {
			t.Fatalf("unsafe id stored: %+v", art)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "evil")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("write escaped the output directory")
	}
}

func TestFilingsDayHelpAndDate(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"filings", "day", "--help"})
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := buf.String()
	for _, want := range []string{"Asia/Manila", "acquisition_cutoff", "corpus_complete", "max-scan-pages", "YYYYMMDD", "Exit 6"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q", want)
		}
	}
	now := manilaDay(2026, 9, 25, 8)
	if _, err := parseDisclosureDay("", now); err != nil {
		t.Fatal(err)
	}
	got, err := parseDisclosureDay("20260924", now)
	if err != nil || got.Format("20060102") != "20260924" {
		t.Fatalf("yyyymmdd = %v %v", got, err)
	}
	got, err = parseDisclosureDay("09-24-2026", now)
	if err != nil || got.Format("20060102") != "20260924" {
		t.Fatalf("mdy = %v %v", got, err)
	}
	if _, err := parseDisclosureDay("20260926", now); err == nil {
		t.Fatal("future date accepted")
	}
	if code := ExitCode(partialFailureErr(errDayPartial)); code != 6 {
		t.Fatalf("partial exit = %d", code)
	}
	if code := ExitCode(apiErr(errDayDiscovery)); code != 5 {
		t.Fatalf("discovery exit = %d", code)
	}
}
