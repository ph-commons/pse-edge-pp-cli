// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFindReportUsesPageTokens(t *testing.T) {
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	var pdfHits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/market-report/":
			w.Write([]byte(listingPage(srv.URL + "/ajax")))
		case "/ajax":
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			for _, want := range []string{"nonce-from-page", "ptp_frompage_1", "ptp_load_posts", "end-of-day-quotes"} {
				if !strings.Contains(text, want) {
					t.Errorf("ajax body missing %s", want)
				}
			}
			if strings.Contains(text, "hardcoded-nonce") {
				t.Error("request used a hardcoded nonce")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(ajaxPayload([]ajaxRow{
				{Title: "September 24, 2026", Categories: `<span data-slug="daily-short-sell-report">Daily Short Sell Report</span>`, Date: "September 24, 2026", Content: `<a href="` + srv.URL + `/short.pdf">x</a>`},
				{Title: "September 23, 2026", Categories: `<span data-slug="end-of-day-quotes">End of Day Quotes</span>`, Date: "September 24, 2026", Content: `<a href="` + srv.URL + `/yesterday.pdf">x</a>`},
				{Title: "September 24, 2026", Categories: `<span data-slug="end-of-day-quotes">End of Day Quotes</span>`, Date: "September 25, 2026", Content: `<a href="` + srv.URL + `/eod.pdf">x</a>`},
			}))
		case "/eod.pdf":
			pdfHits++
			w.Write([]byte("%PDF-1.4\nfixture\n"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	found, err := FindReport(context.Background(), srv.URL+"/market-report/", session)
	if err != nil {
		t.Fatal(err)
	}
	if pdfHits != 1 {
		t.Fatalf("pdf hits=%d", pdfHits)
	}
	if found.Discovery.UploadDate != "September 25, 2026" || found.Discovery.SessionTitle != "September 24, 2026" {
		t.Fatalf("discovery=%+v", found.Discovery)
	}
	if !strings.HasSuffix(found.PDFs[0].URL, "/eod.pdf") || string(found.PDFs[0].Body[:5]) != "%PDF-" {
		t.Fatalf("pdf=%+v", found.PDFs[0].URL)
	}
}

func TestFailedListingDoesNotGuessURL(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	_, err := FindReport(context.Background(), srv.URL+"/market-report/", session)
	if err == nil || statusOf(err) != StatusListingUnavailable {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "EOD.pdf") || strings.Contains(err.Error(), "September%2024") {
		t.Fatalf("error invented a URL: %v", err)
	}
	for _, path := range paths {
		if strings.Contains(path, "EOD") || strings.Contains(path, ".pdf") {
			t.Fatalf("guessed request path %s in %v", path, paths)
		}
	}
}

func TestListingWithoutTargetIsNotYesterday(t *testing.T) {
	var pdfHits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/market-report/" {
			w.Write([]byte(listingPage(srv.URL + "/ajax")))
			return
		}
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			pdfHits++
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(ajaxPayload([]ajaxRow{{
			Title: "September 23, 2026", Categories: `<span data-slug="end-of-day-quotes">End of Day Quotes</span>`, Date: "September 24, 2026", Content: `<a href="` + srv.URL + `/old.pdf">x</a>`,
		}}))
	}))
	defer srv.Close()
	_, err := FindReport(context.Background(), srv.URL+"/market-report/", time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if err == nil || statusOf(err) != StatusReportUnavailable {
		t.Fatalf("err=%v", err)
	}
	if pdfHits != 0 {
		t.Fatalf("downloaded a non-target pdf %d times", pdfHits)
	}
}

func TestLiveQuotationReport(t *testing.T) {
	if os.Getenv("PSE_EDGE_QUOTATION_LIVE") != "1" {
		t.Skip("set PSE_EDGE_QUOTATION_LIVE=1 for one bounded live read")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is required for a live parse")
	}
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	root := t.TempDir()
	started := time.Now()
	report, err := Fetch(ctx, root, DefaultListingURL, session, []string{"AT", "DHI"}, started)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != StatusOK || report.Source == nil || report.Source.SHA256 == "" {
		t.Fatalf("report=%+v", report)
	}
	if len(report.Rows) != 2 || report.Rows[0].Symbol != "AT" || report.Rows[0].Close == nil {
		t.Fatalf("rows=%+v", report.Rows)
	}
	if report.Source.SHA256 == "aa24a2f55058ef8b4dc573091471971351a8c821ba5e9c9698a1afb2874754a0" && math.Abs(*report.Rows[0].Close-19.98) > 1e-6 {
		t.Fatalf("AT close=%v", *report.Rows[0].Close)
	}
	if time.Since(started) > 45*time.Second {
		t.Fatalf("acquisition took %s", time.Since(started))
	}
	t.Logf("acquired_at=%s sha=%s elapsed=%s rows=%d", report.Source.AcquiredAt, report.Source.SHA256, time.Since(started), report.Coverage.RowCount)
}

func ajaxPayload(rows []ajaxRow) []byte {
	raw, err := json.Marshal(map[string]any{"data": rows})
	if err != nil {
		panic(err)
	}
	return raw
}

func listingPage(ajax string) string {
	return `<table id="ptp_frompage_1" class="posts-data-table"></table>
<script>var posts_table_params = {"ajax_url":"` + ajax + `","ajax_nonce":"nonce-from-page","ajax_action":"ptp_load_posts"};</script>`
}
