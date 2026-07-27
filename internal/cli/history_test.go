// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
)

// TestNovelHistoryHelpWires smoke-tests that the history command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelHistoryHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"history", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("history --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "history"} {
		if !strings.Contains(help, want) {
			t.Fatalf("history --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestAnalyticsDateRange(t *testing.T) {
	asOf := time.Date(2026, 7, 27, 0, 0, 0, 0, psecal.Manila())
	window := 30 * 24 * time.Hour

	from, to, err := analyticsDateRange("", "", asOf, window)
	if err != nil || from != "2026-06-27" || to != "2026-07-27" {
		t.Fatalf("default range = %s..%s (err %v), want 2026-06-27..2026-07-27", from, to, err)
	}

	from, to, err = analyticsDateRange("2020-01-01", "2020-01-31", asOf, window)
	if err != nil || from != "2020-01-01" || to != "2020-01-31" {
		t.Fatalf("explicit range = %s..%s (err %v), want 2020-01-01..2020-01-31", from, to, err)
	}

	// --from alone keeps the as-of end; --to alone anchors the window end.
	from, to, err = analyticsDateRange("2026-01-01", "", asOf, window)
	if err != nil || from != "2026-01-01" || to != "2026-07-27" {
		t.Fatalf("from-only range = %s..%s (err %v), want 2026-01-01..2026-07-27", from, to, err)
	}

	if _, _, err = analyticsDateRange("2026-07-28", "2026-07-27", asOf, window); err == nil {
		t.Fatal("inverted range accepted, want error")
	}
	if _, _, err = analyticsDateRange("28-07-2026", "", asOf, window); err == nil {
		t.Fatal("malformed --from accepted, want error")
	}
}
