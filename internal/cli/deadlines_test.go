// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelDeadlinesHelpWires smoke-tests that the deadlines command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelDeadlinesHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"deadlines", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("deadlines --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "deadlines"} {
		if !strings.Contains(help, want) {
			t.Fatalf("deadlines --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestFilingScheduleDueDates pins the SRC due-date arithmetic for all four
// 2026 periods: 45 calendar days after each of the first three
// quarter-ends, 105 days after the Dec 31 fiscal year-end.
func TestFilingScheduleDueDates(t *testing.T) {
	sched := filingSchedule(2026, "2026-07-27")
	want := []struct{ form, periodEnd, due string }{
		{"17-A", "2025-12-31", "2026-04-15"},
		{"17-Q", "2026-03-31", "2026-05-15"},
		{"17-Q", "2026-06-30", "2026-08-14"},
		{"17-Q", "2026-09-30", "2026-11-14"},
	}
	if len(sched) != len(want) {
		t.Fatalf("filingSchedule(2026) returned %d deadlines, want %d: %+v", len(sched), len(want), sched)
	}
	for i, w := range want {
		got := sched[i]
		if got.Form != w.form || got.PeriodEnd != w.periodEnd || got.DueDate != w.due {
			t.Fatalf("deadline[%d] = {%s %s due %s}, want {%s %s due %s}",
				i, got.Form, got.PeriodEnd, got.DueDate, w.form, w.periodEnd, w.due)
		}
	}
}

// TestFilingSchedulePastAllAppendsNext verifies that once every due date
// in the year has passed, the next upcoming deadline (next year's 17-A,
// covering the current fiscal year) is appended.
func TestFilingSchedulePastAllAppendsNext(t *testing.T) {
	sched := filingSchedule(2026, "2026-12-01") // past Nov 14, the year's last due date
	if len(sched) != 5 {
		t.Fatalf("filingSchedule past all due dates returned %d deadlines, want 5: %+v", len(sched), sched)
	}
	next := sched[4]
	if next.Form != "17-A" || next.PeriodEnd != "2026-12-31" || next.DueDate != "2027-04-15" {
		t.Fatalf("appended next deadline = {%s %s due %s}, want {17-A 2026-12-31 due 2027-04-15}",
			next.Form, next.PeriodEnd, next.DueDate)
	}
}

func TestClassifyFiling(t *testing.T) {
	q2 := filingDeadline{Form: "17-Q", PeriodEnd: "2026-06-30", DueDate: "2026-08-14"}
	q1 := filingDeadline{Form: "17-Q", PeriodEnd: "2026-03-31", DueDate: "2026-05-15"}
	annual := filingDeadline{Form: "17-A", PeriodEnd: "2025-12-31", DueDate: "2026-04-15"}
	quarterly := func(date string) deadlineDisclosure {
		return deadlineDisclosure{template: "Quarterly Report", title: "SEC Form 17-Q", disclosedAt: date}
	}

	t.Run("no disclosures at all is unknown, never guessed", func(t *testing.T) {
		got := classifyFiling(q1, []deadlineDisclosure{}, "2026-07-27")
		if got.Status != "unknown" {
			t.Fatalf("status = %q, want unknown", got.Status)
		}
	})

	t.Run("disclosure exactly on due date counts as filed", func(t *testing.T) {
		got := classifyFiling(q1, []deadlineDisclosure{quarterly("2026-05-15")}, "2026-07-27")
		if got.Status != "filed" || got.FiledAt != "2026-05-15" {
			t.Fatalf("status = %q filed_at = %q, want filed on 2026-05-15", got.Status, got.FiledAt)
		}
	})

	t.Run("disclosure at due date plus 30d slack still filed", func(t *testing.T) {
		got := classifyFiling(q1, []deadlineDisclosure{quarterly("2026-06-14")}, "2026-07-27")
		if got.Status != "filed" {
			t.Fatalf("status = %q, want filed (within +30d slack)", got.Status)
		}
	})

	t.Run("disclosure past the slack window does not count", func(t *testing.T) {
		got := classifyFiling(q1, []deadlineDisclosure{quarterly("2026-06-15")}, "2026-07-27")
		if got.Status != "overdue" {
			t.Fatalf("status = %q, want overdue (disclosure outside join window)", got.Status)
		}
	})

	t.Run("disclosure before period end belongs to a prior quarter", func(t *testing.T) {
		got := classifyFiling(q2, []deadlineDisclosure{quarterly("2026-05-15")}, "2026-08-20")
		if got.Status != "overdue" {
			t.Fatalf("status = %q, want overdue (May filing cannot prove the Jun 30 quarter)", got.Status)
		}
	})

	t.Run("not yet due is pending with days_left", func(t *testing.T) {
		got := classifyFiling(q2, []deadlineDisclosure{quarterly("2026-05-15")}, "2026-07-27")
		if got.Status != "pending" {
			t.Fatalf("status = %q, want pending", got.Status)
		}
		if got.DaysLeft == nil || *got.DaysLeft != 18 {
			t.Fatalf("days_left = %v, want 18 (2026-07-27 to 2026-08-14)", got.DaysLeft)
		}
	})

	t.Run("today equal to due date is still pending", func(t *testing.T) {
		got := classifyFiling(q2, []deadlineDisclosure{quarterly("2026-01-05")}, "2026-08-14")
		if got.Status != "pending" || got.DaysLeft == nil || *got.DaysLeft != 0 {
			t.Fatalf("status = %q days_left = %v, want pending with 0", got.Status, got.DaysLeft)
		}
	})

	t.Run("annual matches Annual Report template not Quarterly", func(t *testing.T) {
		discs := []deadlineDisclosure{
			quarterly("2026-04-10"),
			{template: "Annual Report", title: "SEC Form 17-A", disclosedAt: "2026-04-10"},
		}
		got := classifyFiling(annual, discs, "2026-07-27")
		if got.Status != "filed" || got.FiledAt != "2026-04-10" {
			t.Fatalf("status = %q filed_at = %q, want filed on 2026-04-10", got.Status, got.FiledAt)
		}
		onlyQuarterly := classifyFiling(annual, []deadlineDisclosure{quarterly("2026-04-10")}, "2026-07-27")
		if onlyQuarterly.Status != "overdue" {
			t.Fatalf("status = %q, want overdue (a 17-Q cannot prove the 17-A)", onlyQuarterly.Status)
		}
	})

	t.Run("undated disclosure can never prove a filing", func(t *testing.T) {
		got := classifyFiling(q1, []deadlineDisclosure{{template: "Quarterly Report", disclosedAt: ""}}, "2026-07-27")
		if got.Status != "overdue" {
			t.Fatalf("status = %q, want overdue (undated disclosure skipped)", got.Status)
		}
	})
}

func TestNormalizeDisclosureDate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2026-07-25", "2026-07-25"},
		{"2026-07-25 08:31", "2026-07-25"},
		{"2026-07-25T08:31:00+08:00", "2026-07-25"},
		{"Jul 25, 2026 08:31 AM", "2026-07-25"},
		{"Jul 25, 2026 8:31 AM", "2026-07-25"},
		{"Jul 5, 2026 2:52 PM", "2026-07-05"},
		{"Jul 25, 2026", "2026-07-25"},
		{"07-25-2026", "2026-07-25"},
		{"not a date", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeDisclosureDate(tc.in); got != tc.want {
			t.Fatalf("normalizeDisclosureDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
