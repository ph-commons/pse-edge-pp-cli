// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

func TestExportEODJSONL(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "data.db")
	ctx := context.Background()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsurePSEEdgeTables(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPSEEODPrices(ctx, []store.PSEEODRow{
		{Symbol: "AT", TradingDate: "2026-06-01", Open: 1, High: 1, Low: 1, Close: 1, Value: 1, Source: "edge"},
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	root := RootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{
		"export", "eod",
		"--db", dbPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--format", "jsonl",
		"--json", // ensure machine path if needed
	})
	// export uses --format jsonl; --json is global and may force something else — use plain
	root.SetArgs([]string{
		"export", "eod",
		"--db", dbPath,
		"--from", "2026-01-01",
		"--to", "2026-12-31",
		"--format", "jsonl",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr=%s", err, errb.String())
	}
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatalf("empty stdout; stderr=%s", errb.String())
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("json: %v line=%q", err, line)
	}
	if row["contract"] != store.ExportContractEOD {
		t.Fatalf("contract=%v", row["contract"])
	}
	if row["symbol"] != "AT" {
		t.Fatalf("symbol=%v", row["symbol"])
	}
}

func TestExportHelpListsLocalResources(t *testing.T) {
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"export", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"eod", "index", "companies-local", "downstream-integration"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" at, gtcap , ")
	if len(got) != 2 || got[0] != "AT" || got[1] != "GTCAP" {
		t.Fatalf("%v", got)
	}
}
