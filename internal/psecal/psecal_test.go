// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package psecal

import (
	"strings"
	"testing"
	"time"
)

// mnl builds a Manila-local time for tests.
func mnl(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, Manila())
}

func TestIsTradingDay(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"saturday", mnl(2026, time.July, 25, 12, 0), false},
		{"sunday", mnl(2026, time.July, 26, 12, 0), false},
		{"holiday araw ng kagitingan (thu)", mnl(2026, time.April, 9, 12, 0), false},
		{"holiday good friday", mnl(2026, time.April, 3, 12, 0), false},
		{"ordinary monday", mnl(2026, time.July, 27, 12, 0), true},
		{"unknown future weekday defaults to trading", mnl(2027, time.March, 3, 12, 0), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTradingDay(tt.t); got != tt.want {
				t.Errorf("IsTradingDay(%s) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func TestIsTradingDayMachineTZIndependent(t *testing.T) {
	// Friday 2026-07-24 23:00 UTC is Saturday 07:00 in Manila — must be
	// treated as the Manila calendar date (non-trading), not the UTC one.
	utc := time.Date(2026, time.July, 24, 23, 0, 0, 0, time.UTC)
	if IsTradingDay(utc) {
		t.Errorf("IsTradingDay(Fri 23:00 UTC = Sat 07:00 Manila) = true, want false")
	}
}

func TestLastCompletedTradingDay(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{"saturday -> friday", mnl(2026, time.July, 25, 10, 0), "2026-07-24"},
		{"holiday thu 2026-04-09 -> wednesday", mnl(2026, time.April, 9, 12, 0), "2026-04-08"},
		{"monday 09:00 pre-gate -> previous friday", mnl(2026, time.July, 27, 9, 0), "2026-07-24"},
		{"monday 15:59 pre-gate -> previous friday", mnl(2026, time.July, 27, 15, 59), "2026-07-24"},
		{"monday 16:00 at gate -> monday", mnl(2026, time.July, 27, 16, 0), "2026-07-27"},
		{"monday 17:00 post-gate -> monday", mnl(2026, time.July, 27, 17, 0), "2026-07-27"},
		{"good friday holiday -> maundy wed", mnl(2026, time.April, 3, 12, 0), "2026-04-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LastCompletedTradingDay(tt.now).Format("2006-01-02"); got != tt.want {
				t.Errorf("LastCompletedTradingDay(%s) = %s, want %s", tt.now, got, tt.want)
			}
		})
	}
}

func TestNextTradingDay(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"friday -> monday", mnl(2026, time.July, 24, 12, 0), "2026-07-27"},
		{"saturday -> monday", mnl(2026, time.July, 25, 12, 0), "2026-07-27"},
		{"maundy wed -> tue after black saturday+araw", mnl(2026, time.April, 1, 12, 0), "2026-04-06"},
		{"wed 2026-04-08 -> friday (araw ng kagitingan thu skipped)", mnl(2026, time.April, 8, 12, 0), "2026-04-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextTradingDay(tt.t).Format("2006-01-02"); got != tt.want {
				t.Errorf("NextTradingDay(%s) = %s, want %s", tt.t, got, tt.want)
			}
		})
	}
}

func TestSessionState(t *testing.T) {
	tests := []struct {
		name          string
		now           time.Time
		wantTrading   bool
		wantPhase     string
		wantLast      string
		wantNext      string
		wantPolicyCut string // substring the policy must contain
	}{
		{
			name:        "saturday closed",
			now:         mnl(2026, time.July, 25, 11, 0),
			wantTrading: false, wantPhase: "closed",
			wantLast: "2026-07-24", wantNext: "2026-07-27",
			wantPolicyCut: "final",
		},
		{
			name:        "holiday closed",
			now:         mnl(2026, time.April, 9, 11, 0),
			wantTrading: false, wantPhase: "closed",
			wantLast: "2026-04-08", wantNext: "2026-04-10",
			wantPolicyCut: "final",
		},
		{
			name:        "monday 09:00 pre-open provisional friday",
			now:         mnl(2026, time.July, 27, 9, 0),
			wantTrading: true, wantPhase: "pre-open",
			wantLast: "2026-07-24", wantNext: "2026-07-28",
			wantPolicyCut: "provisional",
		},
		{
			name:        "monday 11:00 open provisional",
			now:         mnl(2026, time.July, 27, 11, 0),
			wantTrading: true, wantPhase: "open",
			wantLast: "2026-07-24", wantNext: "2026-07-28",
			wantPolicyCut: "provisional",
		},
		{
			name:        "monday 17:00 post-close final monday",
			now:         mnl(2026, time.July, 27, 17, 0),
			wantTrading: true, wantPhase: "post-close",
			wantLast: "2026-07-27", wantNext: "2026-07-28",
			wantPolicyCut: "final",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := SessionState(tt.now)
			if st.TradingDay != tt.wantTrading {
				t.Errorf("TradingDay = %v, want %v", st.TradingDay, tt.wantTrading)
			}
			if st.Phase != tt.wantPhase {
				t.Errorf("Phase = %q, want %q", st.Phase, tt.wantPhase)
			}
			if st.LastCompleted != tt.wantLast {
				t.Errorf("LastCompleted = %q, want %q", st.LastCompleted, tt.wantLast)
			}
			if st.NextTrading != tt.wantNext {
				t.Errorf("NextTrading = %q, want %q", st.NextTrading, tt.wantNext)
			}
			if !contains(st.AsOfPolicy, tt.wantPolicyCut) {
				t.Errorf("AsOfPolicy = %q, want substring %q", st.AsOfPolicy, tt.wantPolicyCut)
			}
		})
	}
}

func TestTradingDaysBetween(t *testing.T) {
	// Fri 2026-07-17 .. Fri 2026-07-24 exclusive: Mon 20, Tue 21, Wed 22, Thu 23.
	got := TradingDaysBetween("2026-07-17", "2026-07-24")
	want := []string{"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23"}
	if len(got) != len(want) {
		t.Fatalf("TradingDaysBetween = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TradingDaysBetween = %v, want %v", got, want)
		}
	}
	if out := TradingDaysBetween("bogus", "2026-07-24"); len(out) != 0 {
		t.Errorf("malformed input should yield empty slice, got %v", out)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestCalendarCoverage(t *testing.T) {
	minY, maxY, covered := CalendarCoverage(mnl(2026, time.July, 27, 12, 0))
	if minY != 2026 || maxY != 2026 || !covered {
		t.Errorf("2026 coverage = (%d, %d, %v), want (2026, 2026, true)", minY, maxY, covered)
	}
	if _, _, covered := CalendarCoverage(mnl(2027, time.January, 15, 12, 0)); covered {
		t.Error("2027 must be outside holiday-table coverage")
	}
	if _, _, covered := CalendarCoverage(mnl(2025, time.December, 31, 12, 0)); covered {
		t.Error("2025 must be outside holiday-table coverage")
	}
}

func TestSessionStateCalendarWarning(t *testing.T) {
	// Inside coverage: no warning.
	if st := SessionState(mnl(2026, time.July, 27, 17, 0)); st.CalendarWarning != "" {
		t.Errorf("2026 session must carry no calendar warning, got %q", st.CalendarWarning)
	}
	// Outside coverage (2027): explicit warning, never a silent
	// weekday-equals-trading-day assumption.
	st := SessionState(mnl(2027, time.January, 15, 17, 0))
	if st.CalendarWarning == "" {
		t.Fatal("2027 session must carry a calendar_warning")
	}
	for _, cut := range []string{"2026", "2027"} {
		if !strings.Contains(st.CalendarWarning, cut) {
			t.Errorf("calendar_warning %q must mention %s", st.CalendarWarning, cut)
		}
	}
}
