// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestFinalizeFilingsOutStandingWarning(t *testing.T) {
	out := filingsOut{
		Rows: []filingRow{
			{DisclosedAt: "2026-06-30T15:53:00+08:00"},
		},
		ScannedPages: 1,
		TotalPages:   1,
		TotalCount:   19,
		FromDate:     "01-01-2024",
		ToDate:       "08-05-2026",
		Limit:        20,
		MaxScanPages: 3,
	}
	finalizeFilingsOut(&out, false)
	if len(out.Warnings) == 0 || !strings.Contains(out.Warnings[0], "not an authoritative complete corpus") {
		t.Fatalf("missing corpus warning: %v", out.Warnings)
	}
	if out.NewestDisclosedAt != "2026-06-30" {
		t.Errorf("newest = %q", out.NewestDisclosedAt)
	}
	if out.FreshnessGapDays == nil || *out.FreshnessGapDays < freshnessGapWarnDays {
		t.Fatalf("freshness_gap_days = %v, want >= %d", out.FreshnessGapDays, freshnessGapWarnDays)
	}
	foundGap := false
	for _, w := range out.Warnings {
		if strings.Contains(w, "newest search hit") {
			foundGap = true
		}
	}
	if !foundGap {
		t.Fatalf("expected freshness-gap warning, got %v", out.Warnings)
	}
	// LODE-shaped: full page scan of search set, under limit → complete relative to search
	if !out.Complete {
		t.Errorf("complete = false, want true relative to full search set; note=%q truncated=%v page_cap=%v", out.Note, out.Truncated, out.PageCapHit)
	}
	if out.ReturnedCount != 1 {
		t.Errorf("returned_count = %d", out.ReturnedCount)
	}
}

func TestFinalizeFilingsOutPageCap(t *testing.T) {
	out := filingsOut{
		Rows:         []filingRow{{DisclosedAt: "2026-01-01T00:00:00+08:00"}},
		ScannedPages: 3,
		TotalPages:   10,
		TotalCount:   500,
		FromDate:     "01-01-2024",
		ToDate:       "01-05-2024",
		Limit:        20,
		MaxScanPages: 3,
	}
	finalizeFilingsOut(&out, false)
	if out.Complete {
		t.Error("complete should be false when page cap hit")
	}
	if !out.PageCapHit || !out.Truncated {
		t.Errorf("page_cap_hit=%v truncated=%v", out.PageCapHit, out.Truncated)
	}
	if !strings.Contains(out.Note, "page cap") {
		t.Errorf("note = %q", out.Note)
	}
}

func TestFinalizeFilingsOutLimitTruncate(t *testing.T) {
	rows := make([]filingRow, 20)
	for i := range rows {
		rows[i] = filingRow{DisclosedAt: "2026-07-01T00:00:00+08:00"}
	}
	out := filingsOut{
		Rows:         rows,
		ScannedPages: 1,
		TotalPages:   1,
		TotalCount:   50,
		FromDate:     "01-01-2026",
		ToDate:       "07-02-2026",
		Limit:        20,
		MaxScanPages: 3,
	}
	finalizeFilingsOut(&out, false)
	if out.Complete || !out.Truncated {
		t.Errorf("complete=%v truncated=%v", out.Complete, out.Truncated)
	}
	if !strings.Contains(out.Note, "truncated at --limit") {
		t.Errorf("note = %q", out.Note)
	}
}

func TestFilingsGetHelpWired(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"filings", "get", "--help"})
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("filings get --help: %v", err)
	}
	help := buf.String()
	for _, want := range []string{"edge-no", "openDiscViewer"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
