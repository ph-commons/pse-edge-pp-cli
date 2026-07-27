// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// The fixture is the live announcements/search.ax response for
// companyId=633 (GTCAP), 01-01-2026..07-27-2026, captured 2026-07-27:
// pager [1 / 1] [Total 27] with 27 rows.
func gtcapDisclosuresFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/disclosures_search_gtcap.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(data)
}

func TestParseDisclosurePageGTCAP(t *testing.T) {
	page, err := ParseDisclosurePage(gtcapDisclosuresFixture(t))
	if err != nil {
		t.Fatalf("ParseDisclosurePage: %v", err)
	}
	if page.PageNo != 1 || page.TotalPages != 1 || page.TotalCount != 27 {
		t.Errorf("pager = page %d / %d, total %d; want 1 / 1, total 27", page.PageNo, page.TotalPages, page.TotalCount)
	}
	if len(page.Rows) != 27 {
		t.Fatalf("rows = %d, want 27", len(page.Rows))
	}

	first := page.Rows[0]
	if first.EdgeNo != "83eed7f77a89ed3964d70b69f0a3140b" {
		t.Errorf("first edge_no = %q", first.EdgeNo)
	}
	if first.CmpyID != 633 {
		t.Errorf("first cmpy_id = %d, want 633", first.CmpyID)
	}
	if first.Company != "GT Capital Holdings, Inc." {
		t.Errorf("first company = %q", first.Company)
	}
	// Entity-decoded title: source is "Notice of Analysts&#039;/Investors&#039; Briefing".
	if first.Title != "Notice of Analysts'/Investors' Briefing" {
		t.Errorf("first title = %q", first.Title)
	}
	if first.Template != "14-1" {
		t.Errorf("first template = %q, want 14-1", first.Template)
	}
	// "Jul 27, 2026 02:52 PM" Manila -> RFC3339 with +08:00 offset.
	if !strings.HasPrefix(first.DisclosedAt, "2026-07-27T14:52:00") || !strings.HasSuffix(first.DisclosedAt, "+08:00") {
		t.Errorf("first disclosed_at = %q, want 2026-07-27T14:52:00+08:00", first.DisclosedAt)
	}
	if first.CircularNo != "C05666-2026" {
		t.Errorf("first circular_no = %q, want C05666-2026", first.CircularNo)
	}

	// Every row must carry a non-empty edge_no and a 2026 timestamp.
	for i, r := range page.Rows {
		if r.EdgeNo == "" {
			t.Errorf("row %d has empty edge_no", i)
		}
		if !strings.HasPrefix(r.DisclosedAt, "2026-") {
			t.Errorf("row %d disclosed_at = %q, want 2026 date", i, r.DisclosedAt)
		}
	}
}

func TestParseDisclosurePageShell(t *testing.T) {
	_, err := ParseDisclosurePage("<html><body>no pager here</body></html>")
	var shell *ShellPageError
	if !errors.As(err, &shell) {
		t.Fatalf("expected ShellPageError, got %v", err)
	}
}

func TestParseDisclosurePageChallenge(t *testing.T) {
	_, err := ParseDisclosurePage("<html>challenge-platform</html>")
	var ch *ChallengeError
	if !errors.As(err, &ch) {
		t.Fatalf("expected ChallengeError, got %v", err)
	}
}

func TestViewerURL(t *testing.T) {
	got := ViewerURL("83eed7f77a89ed3964d70b69f0a3140b")
	want := "https://edge.pse.com.ph/openDiscViewer.do?edge_no=83eed7f77a89ed3964d70b69f0a3140b"
	if got != want {
		t.Errorf("ViewerURL = %q, want %q", got, want)
	}
}

// disclosureRowPage builds a minimal one-row search page with the given
// timestamp cell content.
func disclosureRowPage(ts string) string {
	return `<span class="count">[1 / 1] [Total 1]</span>
<table><tr>
<td><a href="/companyInformation/form.do?cmpy_id=633">GT Capital Holdings, Inc.</a></td>
<td><a href="#viewer" onclick="openPopup('83eed7f77a89ed3964d70b69f0a3140b');return false;">Notice</a></td>
<td class="alignC">14-1</td>
<td class="alignC">` + ts + `</td>
<td class="alignC">C05666-2026</td>
</tr></table>`
}

func TestParseDisclosurePageTimestampLayouts(t *testing.T) {
	tests := []struct {
		name        string
		ts          string
		wantAt      string // RFC3339 prefix; "" means DisclosedAt must be empty
		wantRawKept bool
	}{
		{name: "zero-padded hour", ts: "Jul 27, 2026 02:52 PM", wantAt: "2026-07-27T14:52:00+08:00"},
		{name: "single-digit hour", ts: "Jul 27, 2026 2:52 PM", wantAt: "2026-07-27T14:52:00+08:00"},
		{name: "24h clock", ts: "Jul 27, 2026 14:52", wantAt: "2026-07-27T14:52:00+08:00"},
		{name: "unparseable leaks to raw_timestamp only", ts: "sometime last Tuesday", wantAt: "", wantRawKept: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ParseDisclosurePage(disclosureRowPage(tc.ts))
			if err != nil {
				t.Fatalf("ParseDisclosurePage: %v", err)
			}
			if len(page.Rows) != 1 {
				t.Fatalf("rows = %d, want 1", len(page.Rows))
			}
			row := page.Rows[0]
			if row.DisclosedAt != tc.wantAt {
				t.Errorf("DisclosedAt = %q, want %q (raw must NEVER land here)", row.DisclosedAt, tc.wantAt)
			}
			if tc.wantRawKept {
				if row.RawTimestamp != tc.ts {
					t.Errorf("RawTimestamp = %q, want %q", row.RawTimestamp, tc.ts)
				}
			} else if row.RawTimestamp != "" {
				t.Errorf("RawTimestamp = %q, want empty on successful parse", row.RawTimestamp)
			}
		})
	}
}
