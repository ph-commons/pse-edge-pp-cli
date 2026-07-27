// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: implemented (was a generated scaffold).
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/psecal"
	"github.com/ngpestelos/pse-edge-pp-cli/internal/store"
)

// Filing rules implemented here (documented, not fetched):
//
//	17-Q  due 45 calendar days after each of the first three quarter-ends
//	      (Mar 31 -> May 15, Jun 30 -> Aug 14, Sep 30 -> Nov 14)
//	17-A  due 105 calendar days after fiscal year-end
//	      (Dec 31 -> Apr 15 in a non-leap following year)
//
// Fiscal-year assumption: CALENDAR year (Dec 31 year-end). Issuers with a
// non-calendar fiscal year will show shifted-looking verdicts; the output
// carries fiscal_year_assumption so downstream consumers see the premise.
const (
	deadlineQuarterlyDueDays = 45
	deadlineAnnualDueDays    = 105
	// deadlineFiledSlackDays widens the join window past the due date: a
	// disclosure dated within [period_end, due_date+30d] still marks the
	// period as filed (late filings happen; the status is about presence,
	// not punctuality).
	deadlineFiledSlackDays = 30
)

// filingDeadline is one computed due-date verdict.
type filingDeadline struct {
	Form      string `json:"form"` // 17-Q | 17-A
	PeriodEnd string `json:"period_end"`
	DueDate   string `json:"due_date"`
	Status    string `json:"status"` // filed | pending | overdue | unknown
	FiledAt   string `json:"filed_at,omitempty"`
	DaysLeft  *int   `json:"days_left,omitempty"`
}

type deadlinesResult struct {
	Symbol               string           `json:"symbol"`
	FiscalYearAssumption string           `json:"fiscal_year_assumption"`
	Deadlines            []filingDeadline `json:"deadlines"`
	Note                 string           `json:"note,omitempty"`
	Source               string           `json:"source"`
	AsOf                 string           `json:"as_of"`
	Stale                bool             `json:"stale"`
}

// deadlineDisclosure is one local disclosure header used for the join.
type deadlineDisclosure struct {
	template    string
	title       string
	disclosedAt string // normalized YYYY-MM-DD; "" when unparseable
}

func newNovelDeadlinesCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "deadlines <SYMBOL>",
		Short: "Computed 17-Q/17-A due dates per SRC Rule 68 45/105-day windows, joined to local disclosures: filed, pending",
		Long: `Use this command for computed quarterly-filing due-date status. Do NOT use it to list raw filings; use 'filings' instead.

Due dates are computed, not fetched: 17-Q is due 45 calendar days after
each of the first three quarter-ends (Mar 31 -> May 15, Jun 30 -> Aug 14,
Sep 30 -> Nov 14); 17-A is due 105 calendar days after the Dec 31 fiscal
year-end (Apr 15 the following non-leap year). Fiscal-year assumption:
CALENDAR year — issuers on a non-calendar fiscal year will not match.

Status joins LOCAL pse_disclosures only (no network): a Quarterly Report
disclosure dated within [period_end, due_date+30d] marks that 17-Q filed;
an Annual Report likewise for the 17-A. Filed exactly on the due date
counts as filed. With no local disclosures for the symbol every status is
'unknown' — run the disclosures sync first; 'pending'/'overdue' are
verdicts against local data, so an unsynced quarter can read as overdue.`,
		Example: `  pse-edge-pp-cli deadlines AT --json
  pse-edge-pp-cli deadlines GTCAP --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("deadlines takes exactly one symbol, got %d arguments", len(args)))
			}
			sym, err := validatePSESymbol(args[0])
			if err != nil {
				return err
			}

			nowManila := time.Now().In(psecal.Manila())
			today := nowManila.Format("2006-01-02")
			asOf := psecal.LastCompletedTradingDay(nowManila).Format("2006-01-02")

			// Missing-mirror guard: typed empty OBJECT (the command's result
			// shape), never a bare [].
			var missing bool
			if dbPath, missing = missingMirrorGuard(cmd, dbPath); missing {
				return printJSONFiltered(cmd.OutOrStdout(), deadlinesResult{
					Symbol:               sym,
					FiscalYearAssumption: "calendar year (Dec 31 fiscal year-end); non-calendar fiscal years are not modeled",
					Deadlines:            []filingDeadline{},
					Note:                 "local store has not been synced yet — run 'pse-edge-pp-cli sync market' first",
					Source:               "local",
					AsOf:                 asOf,
					Stale:                true,
				}, flags)
			}

			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			hintIfUnsynced(cmd, db, "pse_disclosures")
			hintIfStale(cmd, db, "pse_disclosures", 72*time.Hour)

			disclosures, err := deadlineDisclosures(cmd, db, sym)
			if err != nil {
				return err
			}

			schedule := filingSchedule(nowManila.Year(), today)
			for i := range schedule {
				schedule[i] = classifyFiling(schedule[i], disclosures, today)
			}

			res := deadlinesResult{
				Symbol:               sym,
				FiscalYearAssumption: "calendar year (Dec 31 fiscal year-end); non-calendar fiscal years are not modeled",
				Deadlines:            schedule,
				Source:               "local",
				AsOf:                 asOf,
				Stale:                len(disclosures) == 0,
			}
			if len(disclosures) == 0 {
				ytd := fmt.Sprintf("01-01-%d", nowManila.Year())
				res.Note = fmt.Sprintf("no local disclosures for %s — statuses are unknown, not verdicts. Sync disclosures first: run 'pse-edge-pp-cli filings %s --from-date %s'.", sym, sym, ytd)
			}
			return printJSONFiltered(cmd.OutOrStdout(), res, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// filingSchedule computes the year's deadlines (statuses unset): the 17-A
// for the previous fiscal year plus the three 17-Qs. When today is past
// every due date in the year, the next upcoming deadline (next year's
// 17-A, covering the current fiscal year) is appended so the command
// always surfaces what is due next.
func filingSchedule(year int, today string) []filingDeadline {
	sched := []filingDeadline{
		deadlineFor("17-A", time.Date(year-1, 12, 31, 0, 0, 0, 0, time.UTC), deadlineAnnualDueDays),
		deadlineFor("17-Q", time.Date(year, 3, 31, 0, 0, 0, 0, time.UTC), deadlineQuarterlyDueDays),
		deadlineFor("17-Q", time.Date(year, 6, 30, 0, 0, 0, 0, time.UTC), deadlineQuarterlyDueDays),
		deadlineFor("17-Q", time.Date(year, 9, 30, 0, 0, 0, 0, time.UTC), deadlineQuarterlyDueDays),
	}
	pastAll := true
	for _, d := range sched {
		if today <= d.DueDate {
			pastAll = false
			break
		}
	}
	if pastAll {
		sched = append(sched, deadlineFor("17-A", time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC), deadlineAnnualDueDays))
	}
	return sched
}

// deadlineFor computes one deadline: due = period end + dueDays calendar days.
func deadlineFor(form string, periodEnd time.Time, dueDays int) filingDeadline {
	return filingDeadline{
		Form:      form,
		PeriodEnd: periodEnd.Format("2006-01-02"),
		DueDate:   periodEnd.AddDate(0, 0, dueDays).Format("2006-01-02"),
	}
}

// classifyFiling resolves one deadline's status against the local
// disclosure headers. Zero disclosures for the symbol means the local
// mirror cannot support any verdict: status is "unknown", never a guessed
// "pending"/"overdue".
func classifyFiling(d filingDeadline, disclosures []deadlineDisclosure, today string) filingDeadline {
	if len(disclosures) == 0 {
		d.Status = "unknown"
		return d
	}
	matchTerm := "quarterly report"
	if d.Form == "17-A" {
		matchTerm = "annual report"
	}
	windowEnd := addDaysISO(d.DueDate, deadlineFiledSlackDays)
	for _, disc := range disclosures {
		if disc.disclosedAt == "" {
			continue
		}
		hay := strings.ToLower(disc.template + " " + disc.title)
		if !strings.Contains(hay, matchTerm) && !strings.Contains(hay, strings.ToLower(d.Form)) {
			continue
		}
		// Inclusive window: a disclosure exactly on the due date is filed.
		if disc.disclosedAt >= d.PeriodEnd && disc.disclosedAt <= windowEnd {
			d.Status = "filed"
			if d.FiledAt == "" || disc.disclosedAt < d.FiledAt {
				d.FiledAt = disc.disclosedAt
			}
		}
	}
	if d.Status == "filed" {
		return d
	}
	if today <= d.DueDate {
		d.Status = "pending"
		if left, ok := daysBetweenISO(today, d.DueDate); ok {
			d.DaysLeft = &left
		}
		return d
	}
	d.Status = "overdue"
	return d
}

// deadlineDisclosures reads the symbol's local disclosure headers. A
// missing table (disclosures never synced) is an empty slice, not an error.
func deadlineDisclosures(cmd *cobra.Command, db *store.Store, sym string) ([]deadlineDisclosure, error) {
	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT COALESCE(template, ''), COALESCE(title, ''), COALESCE(disclosed_at, '')
		 FROM pse_disclosures WHERE symbol = ?`, sym)
	if err != nil {
		if syncHintMissingTable(err) {
			return []deadlineDisclosure{}, nil
		}
		return nil, err
	}
	out := make([]deadlineDisclosure, 0)
	for rows.Next() {
		var d deadlineDisclosure
		var raw string
		if err := rows.Scan(&d.template, &d.title, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		d.disclosedAt = normalizeDisclosureDate(raw)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// normalizeDisclosureDate extracts a YYYY-MM-DD date from the stored
// disclosed_at value. Unparseable values normalize to "" and are skipped
// by the join (an undated disclosure can never prove a filing).
func normalizeDisclosureDate(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	for _, layout := range []string{"Jan 2, 2006 03:04 PM", "Jan 2, 2006 3:04 PM", "Jan 2, 2006 15:04", "Jan 2, 2006", "01-02-2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// addDaysISO adds n calendar days to a YYYY-MM-DD date.
func addDaysISO(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// daysBetweenISO returns the whole calendar days from a to b (both
// YYYY-MM-DD, b >= a expected).
func daysBetweenISO(a, b string) (int, bool) {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return int(tb.Sub(ta).Hours() / 24), true
}
