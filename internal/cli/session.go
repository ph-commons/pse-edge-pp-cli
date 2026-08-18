// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source computed

package cli

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/ph-commons/pse-edge-pp-cli/internal/psecal"
)

func newNovelSessionCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "session",
		Short: "Philippine trading-calendar verdict: trading day or not, pre/post-close gate, last completed trading date.",
		Long: `Use this command to check trading-day/session state before trusting EOD figures. Do NOT use it for index levels or breadth; use 'market' instead.

Pure local computation over the PH trading calendar (weekends + best-effort
holiday table) with the 16:00 Asia/Manila close gate. All times are evaluated
in Asia/Manila regardless of the machine timezone.

Output fields:
  trading_day    whether today (Manila) is a trading day
  phase          pre-open | open | post-close | closed
  last_completed the most recent trading date whose session has completed
  next_trading   the next trading date after today
  as_of_policy   whether EOD figures for today are final or provisional`,
		Example:     "  pse-edge-pp-cli session --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			state := psecal.SessionState(time.Now())
			return printJSONFiltered(cmd.OutOrStdout(), state, flags)
		},
	}
	return cmd
}
