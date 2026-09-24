// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestEnforceChangePair(t *testing.T) {
	fp := func(f float64) *float64 { return &f }
	cases := []struct {
		name    string
		change  *float64
		pct     *float64
		wantPct *float64
	}{
		{"null_change_zero_pct", nil, fp(0), nil},
		{"null_change_negative_pct", nil, fp(-1.79), nil},
		{"present_change_keeps_pct", fp(0.5), fp(1.2), fp(1.2)},
		{"both_null", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := &quoteRow{Change: tc.change, ChangePct: tc.pct}
			enforceChangePair(row)
			if (row.ChangePct == nil) != (tc.wantPct == nil) {
				t.Fatalf("ChangePct = %v, want %v", row.ChangePct, tc.wantPct)
			}
			if tc.wantPct != nil && *row.ChangePct != *tc.wantPct {
				t.Errorf("ChangePct = %v, want %v", *row.ChangePct, *tc.wantPct)
			}
		})
	}
}
