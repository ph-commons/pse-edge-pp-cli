// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelMoversHelpWires smoke-tests that the movers command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelMoversHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"movers", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("movers --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "movers"} {
		if !strings.Contains(help, want) {
			t.Fatalf("movers --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestRankMovers(t *testing.T) {
	entries := []moverEntry{
		{Symbol: "AAA", Pct: 20},
		{Symbol: "BBB", Pct: -5},
		{Symbol: "CCC", Pct: 3},
		{Symbol: "DDD", Pct: -12},
		{Symbol: "EEE", Pct: 20}, // tie with AAA; symbol breaks the tie
	}
	gainers, losers := rankMovers(entries, 2)
	if len(gainers) != 2 || gainers[0].Symbol != "AAA" || gainers[1].Symbol != "EEE" {
		t.Fatalf("gainers = %+v, want [AAA EEE] (desc by pct, symbol tiebreak)", gainers)
	}
	if len(losers) != 2 || losers[0].Symbol != "DDD" || losers[1].Symbol != "BBB" {
		t.Fatalf("losers = %+v, want [DDD BBB] (asc by pct)", losers)
	}

	// Sign filter: a positive-only universe yields no losers (a +20% entry
	// must never appear under losers, however small the universe).
	gainers, losers = rankMovers(entries[:1], 10)
	if len(gainers) != 1 || len(losers) != 0 {
		t.Fatalf("small-universe rank = %d gainers / %d losers, want 1/0", len(gainers), len(losers))
	}

	// Flat symbols (pct == 0) appear in neither list.
	gainers, losers = rankMovers([]moverEntry{{Symbol: "FLT", Pct: 0}}, 10)
	if len(gainers) != 0 || len(losers) != 0 {
		t.Fatalf("flat rank = %d gainers / %d losers, want 0/0", len(gainers), len(losers))
	}

	gainers, losers = rankMovers(nil, 10)
	if len(gainers) != 0 || len(losers) != 0 {
		t.Fatalf("empty rank = %d gainers / %d losers, want 0/0", len(gainers), len(losers))
	}
}
