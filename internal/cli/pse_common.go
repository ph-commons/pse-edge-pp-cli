// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Shared helpers for the hand-authored PSE Edge porcelain: stale-semantics,
// --since window parsing, and the missing-local-mirror guard. Kept out of
// generated files so `generate --force` never touches them.

package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
)

// liveFetchStale is the single stale-semantics rule for live-fetched data:
// during a trading day before the 16:00 Asia/Manila close gate the page can
// still move, so the answer is provisional (stale:true); post-close on a
// trading day — or on a non-trading day, when the page shows the last
// completed session — it is final. Mirrors quote.go's original inline rule.
func liveFetchStale(state psecal.State) bool {
	return state.TradingDay && state.Phase != "post-close"
}

// parseSinceWindow parses a --since flag value ("30d", "12w", ...) into a
// positive duration, defaulting to def when the flag is empty. Returns a
// usage-typed error (exit 2) on malformed or non-positive values.
func parseSinceWindow(since string, def time.Duration) (time.Duration, error) {
	if since == "" {
		return def, nil
	}
	d, err := cliutil.ParseDurationLoose(since)
	if err != nil {
		return 0, usageErr(fmt.Errorf("invalid --since value %q: %w", since, err))
	}
	if d <= 0 {
		return 0, usageErr(fmt.Errorf("invalid --since value %q: must be a positive window", since))
	}
	return d, nil
}

// missingMirrorGuard resolves the effective database path and reports
// whether the local mirror is missing entirely (no database file — nothing
// has ever been synced). When missing it emits the ONE canonical hint on
// stderr; callers then emit their command's typed empty result on stdout.
func missingMirrorGuard(cmd *cobra.Command, dbPath string) (string, bool) {
	if dbPath == "" {
		dbPath = defaultDBPath("pse-edge-pp-cli")
	}
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: local store has not been synced yet. Run 'pse-edge-pp-cli sync market' before trusting local results.")
		return dbPath, true
	}
	return dbPath, false
}
