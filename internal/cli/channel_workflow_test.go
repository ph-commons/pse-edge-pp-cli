// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

func TestWorkflowStatusDoesNotCreateMissingStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "data.db")
	cmd := newWorkflowStatusCmd(&rootFlags{})
	cmd.SetArgs([]string{"--db", path})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "uninitialized") {
		t.Fatalf("status on missing store = %v, want an uninitialized-store error", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing store was created or stat failed: %v", err)
	}
}

func TestWorkflowStatusFullInvocationLeavesCurrentStoreUnchanged(t *testing.T) {
	t.Setenv("PSE_EDGE_HOME", t.TempDir())
	t.Setenv("PSE_EDGE_NO_LEARN", "")
	path := defaultDBPath("pse-edge-pp-cli")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldArgs := os.Args
	os.Args = []string{"pse-edge-pp-cli", "workflow", "status", "--no-learn"}
	t.Cleanup(func() { os.Args = oldArgs })
	if err := Execute(); err != nil {
		t.Fatalf("status invocation: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("current store changed, read error: %v", err)
	}
}

func TestWorkflowStatusDoesNotMigrateOlderStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newWorkflowStatusCmd(&rootFlags{})
	cmd.SetArgs([]string{"--db", path})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "migration required") {
		t.Fatalf("status on older store = %v, want migration-required error", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("older store changed, read error: %v", err)
	}
}
