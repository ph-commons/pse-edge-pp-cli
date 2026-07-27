// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestValidatePSESymbol pins the ticker-shape gate: symbols are normalized
// (trim + upper-case) and must match ^[A-Z0-9][A-Z0-9.\-]{0,9}$ before they
// can reach a URL or query; anything else is a usage error.
func TestValidatePSESymbol(t *testing.T) {
	valid := []struct{ in, want string }{
		{"AT", "AT"},
		{"at", "AT"},
		{" gtcap ", "GTCAP"},
		{"2GO", "2GO"},
		{"AC-PR", "AC-PR"},
		{"X.Y", "X.Y"},
		{"ABCDEFGHIJ", "ABCDEFGHIJ"}, // 10 chars, at the cap
	}
	for _, tc := range valid {
		got, err := validatePSESymbol(tc.in)
		if err != nil {
			t.Errorf("validatePSESymbol(%q) error = %v, want %q", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("validatePSESymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	invalid := []string{
		"",
		"   ",
		"-AT",           // must start alphanumeric
		".AT",           // must start alphanumeric
		"ABCDEFGHIJK",   // 11 chars, over the cap
		"A T",           // inner space
		"AT;DROP",       // shell/SQL metacharacters
		"../etc/passwd", // path traversal shape
		"AT/../X",
		"AT%2F",
	}
	for _, in := range invalid {
		if got, err := validatePSESymbol(in); err == nil {
			t.Errorf("validatePSESymbol(%q) = %q, want usage error", in, got)
		}
	}
}
