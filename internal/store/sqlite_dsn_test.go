// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDBPathRejectsURIMeta(t *testing.T) {
	for _, p := range []string{
		"/tmp/x.db?mode=rwc",
		"/tmp/x.db#frag",
		"/tmp/a&b.db",
		"file:/tmp/x.db",
		"",
	} {
		if err := validateDBPath(p); err == nil {
			t.Errorf("validateDBPath(%q) = nil, want error", p)
		}
	}
	if err := validateDBPath(filepath.Join(t.TempDir(), "ok.db")); err != nil {
		t.Fatal(err)
	}
}

func TestSqliteDSNModeRoCannotBeOverriddenByPath(t *testing.T) {
	// Rejection is the contract: poisoned paths never reach sql.Open.
	_, err := sqliteDSN("/tmp/evil.db?mode=rwc", "mode=ro&_pragma=busy_timeout(5000)")
	if err == nil {
		t.Fatal("expected reject of path with ?")
	}
	dsn, err := sqliteDSN("/tmp/safe.db", "mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dsn, "file:") || !strings.Contains(dsn, "mode=ro") {
		t.Fatalf("dsn = %q", dsn)
	}
	// Exactly one mode= query key
	if strings.Count(dsn, "mode=") != 1 {
		t.Fatalf("expected single mode=, got %q", dsn)
	}
}
