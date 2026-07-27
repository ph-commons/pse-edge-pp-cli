// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Package psecal models the Philippine Stock Exchange trading calendar.
//
// All computations are performed in Asia/Manila regardless of the machine's
// local timezone. The EOD close gate is 16:00 Manila: data observed before
// the gate on a trading day is provisional (the day is not yet a completed
// trading session); at or after the gate the day is complete.
//
// Holiday knowledge is a best-effort static table (see holidays below).
// Unknown future dates default to "weekday = trading day", which matches the
// binding rule in the red-team contract: the calendar must never invent
// closures it cannot source.
package psecal

import (
	"fmt"
	"time"
)

// CloseGateHour/CloseGateMinute define the 16:00 Asia/Manila EOD close gate.
const (
	CloseGateHour   = 16
	CloseGateMinute = 0
)

// openHour/openMinute is the PSE continuous-trading open (09:30 Manila).
// Used only for the pre-open/open phase split in SessionState; the close
// gate above is what drives EOD as-of attribution.
const (
	openHour   = 9
	openMinute = 30
)

// manila is the exchange timezone. Loaded once; falls back to a fixed
// UTC+8 zone when the tzdata lookup fails (e.g. stripped containers) —
// the Philippines has no DST, so the fixed offset is exact.
var manila = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
		return loc
	}
	return time.FixedZone("PHT", 8*60*60)
}()

// Manila returns the exchange timezone.
func Manila() *time.Location { return manila }

// holidays is the best-effort table of Philippine non-working days on which
// the PSE is closed, keyed by YYYY-MM-DD (Manila date).
//
// Source: Proclamation No. 986 (s. 2025) declaring the regular holidays and
// special (non-working) days for 2026, as published by the Official Gazette,
// plus the customary PSE year-end trading breaks (Dec 24 and Dec 31, which
// the exchange observes even when not proclaimed). The two Islamic holidays
// (Eid'l Fitr, Eid'l Adha) are proclaimed close to the date once the lunar
// sighting is confirmed; the entries below are the projected dates and are
// marked as estimates. Dates falling on weekends are omitted (weekend logic
// already covers them). Unknown dates default to trading.
var holidays = map[string]bool{
	// 2026 regular holidays
	"2026-01-01": true, // New Year's Day (Thu)
	"2026-04-02": true, // Maundy Thursday
	"2026-04-03": true, // Good Friday
	"2026-04-09": true, // Araw ng Kagitingan (Thu)
	"2026-05-01": true, // Labor Day (Fri)
	"2026-06-12": true, // Independence Day (Fri)
	"2026-08-31": true, // National Heroes Day (last Mon of Aug)
	"2026-11-30": true, // Bonifacio Day (Mon)
	"2026-12-25": true, // Christmas Day (Fri)
	"2026-12-30": true, // Rizal Day (Wed)
	// 2026 special non-working days
	"2026-02-17": true, // Chinese New Year (Tue)
	"2026-02-25": true, // EDSA People Power Anniversary (Wed)
	"2026-03-20": true, // Eid'l Fitr (Fri) — estimate, confirmed by proclamation
	"2026-05-27": true, // Eid'l Adha (Wed) — estimate, confirmed by proclamation
	"2026-08-21": true, // Ninoy Aquino Day (Fri)
	"2026-11-02": true, // All Souls' Day special (Mon)
	"2026-12-08": true, // Immaculate Conception (Tue)
	"2026-12-24": true, // Christmas Eve (Thu) — PSE trading break
	"2026-12-31": true, // Last day of the year (Thu) — PSE trading break
}

// minKnownYear/maxKnownYear bound the holiday table above. They MUST be
// updated together with the table when a new year's proclamation is added —
// outside this range only weekend logic applies, so holiday closures would
// silently read as trading days. CalendarCoverage/SessionState surface that
// as an explicit warning instead of fabricating unproclaimed holidays.
const (
	minKnownYear = 2026
	maxKnownYear = 2026
)

// CalendarCoverage reports the holiday-table year bounds and whether now's
// Manila year falls inside them.
func CalendarCoverage(now time.Time) (minYear, maxYear int, covered bool) {
	y := now.In(manila).Year()
	return minKnownYear, maxKnownYear, y >= minKnownYear && y <= maxKnownYear
}

// State is the session verdict returned by SessionState.
type State struct {
	TradingDay      bool   `json:"trading_day"`
	Phase           string `json:"phase"` // pre-open | open | post-close | closed
	LastCompleted   string `json:"last_completed"`
	NextTrading     string `json:"next_trading"`
	AsOfPolicy      string `json:"as_of_policy"`
	CalendarWarning string `json:"calendar_warning,omitempty"`
}

// dateKey renders t's Manila calendar date as YYYY-MM-DD.
func dateKey(t time.Time) string {
	return t.In(manila).Format("2006-01-02")
}

// IsTradingDay reports whether t's Manila calendar date is a PSE trading
// day: weekends are never trading days; known holidays are closed; any
// other weekday defaults to trading.
func IsTradingDay(t time.Time) bool {
	mt := t.In(manila)
	switch mt.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !holidays[dateKey(mt)]
}

// LastCompletedTradingDay returns the Manila date (midnight, Asia/Manila)
// of the most recent trading day whose session has completed as of now:
// today when today is a trading day and now is at/after the 16:00 close
// gate, otherwise the closest prior trading day.
func LastCompletedTradingDay(now time.Time) time.Time {
	mt := now.In(manila)
	day := time.Date(mt.Year(), mt.Month(), mt.Day(), 0, 0, 0, 0, manila)
	gate := day.Add(time.Duration(CloseGateHour)*time.Hour + time.Duration(CloseGateMinute)*time.Minute)
	if IsTradingDay(day) && !mt.Before(gate) {
		return day
	}
	for {
		day = day.AddDate(0, 0, -1)
		if IsTradingDay(day) {
			return day
		}
	}
}

// NextTradingDay returns the Manila date of the first trading day strictly
// after t's Manila calendar date.
func NextTradingDay(t time.Time) time.Time {
	mt := t.In(manila)
	day := time.Date(mt.Year(), mt.Month(), mt.Day(), 0, 0, 0, 0, manila)
	for {
		day = day.AddDate(0, 0, 1)
		if IsTradingDay(day) {
			return day
		}
	}
}

// SessionState computes the full session verdict for now.
func SessionState(now time.Time) State {
	mt := now.In(manila)
	day := time.Date(mt.Year(), mt.Month(), mt.Day(), 0, 0, 0, 0, manila)
	trading := IsTradingDay(day)
	last := LastCompletedTradingDay(now)

	phase := "closed"
	if trading {
		open := day.Add(time.Duration(openHour)*time.Hour + time.Duration(openMinute)*time.Minute)
		gate := day.Add(time.Duration(CloseGateHour)*time.Hour + time.Duration(CloseGateMinute)*time.Minute)
		switch {
		case mt.Before(open):
			phase = "pre-open"
		case mt.Before(gate):
			phase = "open"
		default:
			phase = "post-close"
		}
	}

	policy := "final: EOD as-of " + dateKey(last) + " is a completed session"
	if trading && phase != "post-close" {
		policy = "provisional: today's session is not complete before 16:00 Asia/Manila; EOD as-of = " + dateKey(last)
	}

	st := State{
		TradingDay:    trading,
		Phase:         phase,
		LastCompleted: dateKey(last),
		NextTrading:   dateKey(NextTradingDay(now)),
		AsOfPolicy:    policy,
	}
	if minY, maxY, covered := CalendarCoverage(now); !covered {
		st.CalendarWarning = fmt.Sprintf(
			"holiday table covers %d-%d only; %d holidays are unknown, so non-weekend closures may be reported as trading days",
			minY, maxY, mt.Year())
	}
	return st
}

// TradingDaysBetween returns every trading day strictly between the Manila
// dates from and to (exclusive on both ends), formatted YYYY-MM-DD. Used by
// freshness audits to list in-series gaps. from/to are parsed as YYYY-MM-DD;
// malformed or inverted inputs yield an empty slice.
func TradingDaysBetween(from, to string) []string {
	start, err1 := time.ParseInLocation("2006-01-02", from, manila)
	end, err2 := time.ParseInLocation("2006-01-02", to, manila)
	out := make([]string, 0)
	if err1 != nil || err2 != nil || !start.Before(end) {
		return out
	}
	for d := start.AddDate(0, 0, 1); d.Before(end); d = d.AddDate(0, 0, 1) {
		if IsTradingDay(d) {
			out = append(out, d.Format("2006-01-02"))
		}
	}
	return out
}
