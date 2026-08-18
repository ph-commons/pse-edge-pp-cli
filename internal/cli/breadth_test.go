// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

// TestNovelBreadthHelpWires smoke-tests that the breadth command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelBreadthHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"breadth", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("breadth --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "breadth"} {
		if !strings.Contains(help, want) {
			t.Fatalf("breadth --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestBreadthAdvDecRatio pins the divide-by-zero contract: declines = 0
// yields null, never a fabricated number.
func TestBreadthAdvDecRatio(t *testing.T) {
	if r := breadthAdvDecRatio(120, 60); r == nil || *r != 2.0 {
		t.Fatalf("breadthAdvDecRatio(120, 60) = %v, want 2.0", r)
	}
	if r := breadthAdvDecRatio(120, 0); r != nil {
		t.Fatalf("breadthAdvDecRatio(120, 0) = %v, want nil (declines=0)", *r)
	}
}

func TestBreadthSummarize(t *testing.T) {
	two, one := 2.0, 1.0
	rows := []breadthRow{
		{Advances: 120, Declines: 60, AdvDecRatio: &two}, // advancing
		{Advances: 60, Declines: 60, AdvDecRatio: &one},  // flat
		{Advances: 50, Declines: 100, AdvDecRatio: nil},  // declining, ratio missing
	}
	rows[2].AdvDecRatio = breadthAdvDecRatio(50, 100)
	s := breadthSummarize(rows)
	if s.Days != 3 || s.AdvancingDays != 1 || s.DecliningDays != 1 {
		t.Fatalf("summary = %+v, want days=3 advancing=1 declining=1", s)
	}
	if s.AvgRatio == nil || *s.AvgRatio != (2.0+1.0+0.5)/3 {
		t.Fatalf("avg_ratio = %v, want %v", s.AvgRatio, (2.0+1.0+0.5)/3)
	}
	empty := breadthSummarize(nil)
	if empty.Days != 0 || empty.AvgRatio != nil {
		t.Fatalf("empty summary = %+v, want zero days and nil avg_ratio", empty)
	}
}

// TestBreadthWindowRowsRequiresBothBreadthInts pins the NULL-coercion fix:
// a row with advances set but declines NULL must be excluded entirely —
// never rendered with declines 0 and counted as an advancing day.
func TestBreadthWindowRowsRequiresBothBreadthInts(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if err := s.EnsurePSEEdgeTables(ctx); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	adv1, dec1 := int64(94), int64(75)
	adv2 := int64(80)
	rows := []store.PSEIndexSnapshotRow{
		// Full breadth: included.
		{IndexCode: "PSEI", TradingDate: "2026-07-23", Value: 6300, Advances: &adv1, Declines: &dec1, Source: "edge"},
		// advances present, declines NULL: must be EXCLUDED, not counted
		// as an advancing day with declines 0.
		{IndexCode: "PSEI", TradingDate: "2026-07-24", Value: 6310, Advances: &adv2, Source: "edge"},
		// Close-only backfill row: excluded.
		{IndexCode: "PSEI", TradingDate: "2026-07-27", Value: 6314.9, Source: "edge"},
	}
	if err := s.UpsertPSEIndexSnapshots(ctx, rows); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	got, err := breadthWindowRows(cmd, s, "2026-07-01", "2026-07-31")
	if err != nil {
		t.Fatalf("breadthWindowRows: %v", err)
	}
	if len(got) != 1 || got[0].Date != "2026-07-23" {
		t.Fatalf("rows = %+v, want only the 2026-07-23 full-breadth session", got)
	}
	sum := breadthSummarize(got)
	if sum.Days != 1 || sum.AdvancingDays != 1 || sum.DecliningDays != 0 {
		t.Fatalf("summary = %+v, want days=1 advancing=1 declining=0", sum)
	}

	cov, err := breadthCoverageSpan(cmd, s)
	if err != nil {
		t.Fatalf("breadthCoverageSpan: %v", err)
	}
	if cov == nil || cov.Days != 1 || cov.First != "2026-07-23" || cov.Last != "2026-07-23" {
		t.Fatalf("coverage = %+v, want the single full-breadth session", cov)
	}
}
