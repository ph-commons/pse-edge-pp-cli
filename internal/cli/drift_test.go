// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// TestNovelDriftHelpWires smoke-tests that the drift command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelDriftHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"drift", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("drift --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "drift"} {
		if !strings.Contains(help, want) {
			t.Fatalf("drift --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestDriftPctChange(t *testing.T) {
	cases := []struct {
		name       string
		start, end float64
		want       float64
	}{
		{"rally 8 to 12", 8, 12, 50},
		{"flat", 10, 10, 0},
		{"decline", 10, 8, -20},
		{"small move", 100, 101.5, 1.5},
		{"psei-scale", 6000, 6300, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := driftPctChange(tc.start, tc.end)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("driftPctChange(%v, %v) = %v, want %v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

// TestDriftBandPosition covers the 52-week band math including the
// parenthesized edge cases from the spec: end_close exactly at the 52w
// high must report band_position_pct = 100, and thin local coverage must
// yield null with an honest note rather than a partial-window number.
func TestDriftBandPosition(t *testing.T) {
	cases := []struct {
		name             string
		endClose         float64
		low, high        float64
		coverDays        int
		wantVal          *float64
		wantNoteFragment string
	}{
		{"end at 52w high is 100", 12.5, 7.5, 12.5, 250, driftTestPtr(100), ""},
		{"end at 52w low is 0", 7.5, 7.5, 12.5, 250, driftTestPtr(0), ""},
		{"midpoint is 50", 10, 7.5, 12.5, 250, driftTestPtr(50), ""},
		{"exactly min coverage is allowed", 10, 7.5, 12.5, 200, driftTestPtr(50), ""},
		{"below min coverage is null with note", 10, 7.5, 12.5, 199, nil, "band_position_pct null"},
		{"zero coverage is null with note", 10, 7.5, 12.5, 0, nil, "band_position_pct null"},
		{"degenerate band is null", 10, 10, 10, 250, nil, "degenerate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, note := driftBandPosition(tc.endClose, tc.low, tc.high, tc.coverDays, driftMinBandCoverDays)
			if (got == nil) != (tc.wantVal == nil) {
				t.Fatalf("driftBandPosition(...) = %v, want %v (note %q)", driftTestFmt(got), driftTestFmt(tc.wantVal), note)
			}
			if got != nil && math.Abs(*got-*tc.wantVal) > 1e-9 {
				t.Fatalf("driftBandPosition(...) = %v, want %v", *got, *tc.wantVal)
			}
			if tc.wantNoteFragment != "" && !strings.Contains(note, tc.wantNoteFragment) {
				t.Fatalf("note %q missing fragment %q", note, tc.wantNoteFragment)
			}
			if tc.wantNoteFragment == "" && note != "" {
				t.Fatalf("unexpected note %q", note)
			}
		})
	}
}

// TestDriftRelativeIsDifference pins relative_pct = change_pct - psei_change_pct.
func TestDriftRelativeIsDifference(t *testing.T) {
	change := driftPctChange(8, 12)    // +50
	psei := driftPctChange(6000, 6300) // +5
	if rel := change - psei; math.Abs(rel-45) > 1e-9 {
		t.Fatalf("relative_pct = %v, want 45", rel)
	}
}

func driftTestPtr(v float64) *float64 { return &v }

func driftTestFmt(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
