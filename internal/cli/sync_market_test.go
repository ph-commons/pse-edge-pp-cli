// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/ph-commons/pse-edge-pp-cli/internal/pseedge"
)

// TestCompositeSnapshotDate pins the finality rule for the composite page's
// own trade stamp: a session at or before the last completed trading day is
// storable under its own date; a later stamp is the in-progress session and
// must not be written (issue #53).
func TestCompositeSnapshotDate(t *testing.T) {
	comp := func(tradeDate string) *pseedge.Composite {
		return &pseedge.Composite{Indices: []pseedge.Index{{Code: "PSEI", TradeDate: tradeDate}}}
	}

	cases := []struct {
		name          string
		comp          *pseedge.Composite
		lastCompleted string
		wantDate      string
		wantFinal     bool
	}{
		{
			name:          "equal to last completed",
			comp:          comp("2026-09-23T15:12:00+08:00"),
			lastCompleted: "2026-09-23",
			wantDate:      "2026-09-23",
			wantFinal:     true,
		},
		{
			name:          "earlier completed session",
			comp:          comp("2026-09-22T15:12:00+08:00"),
			lastCompleted: "2026-09-23",
			wantDate:      "2026-09-22",
			wantFinal:     true,
		},
		{
			name:          "in-progress session",
			comp:          comp("2026-09-24T10:05:00+08:00"),
			lastCompleted: "2026-09-23",
			wantDate:      "2026-09-24",
			wantFinal:     false,
		},
		{
			name:          "offset-less stamp is Manila wall time",
			comp:          comp("2026-09-23"),
			lastCompleted: "2026-09-23",
			wantDate:      "2026-09-23",
			wantFinal:     true,
		},
		{
			name:          "no PSEI reading",
			comp:          &pseedge.Composite{},
			lastCompleted: "2026-09-23",
			wantDate:      "",
			wantFinal:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			date, _, ok := compositeSnapshotDate(tc.comp, tc.lastCompleted)
			if date != tc.wantDate || ok != tc.wantFinal {
				t.Fatalf("compositeSnapshotDate = (%q, %v), want (%q, %v)", date, ok, tc.wantDate, tc.wantFinal)
			}
		})
	}
}
