// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Financial-reports parser for /companyPage/financial_reports_view.do?cmpy_id=N.
// Live-verified 2026-07-27 against GTCAP (cmpy_id=633). Page shape:
//
//	<h3>Annual</h3>
//	For the fiscal year ended : Dec 31, 2025
//	Currency(and units, if applicable) : In million pesos
//	<table class="view"><caption>Balance Sheet</caption>
//	  <tr><th>Total Assets</th><td class="alignR">519,043</td><td class="alignR">474,088</td></tr> ...
//	<table class="view"><caption>Income Statement</caption> ...
//	<h3>Quarterly</h3> (same shape; columns are Period Ended / Fiscal Year Ended(Audited))
//
// Values are printed in the units the company filed (thousands/millions) —
// both the printed string and the parsed float are preserved. Parenthesized
// values are negative.

package pseedge

import (
	"fmt"
	"regexp"
	"strings"
)

// NoFinancialsError is returned when the page renders but carries no
// financial tables (company has not published statements on Edge). Typed so
// callers exit with a named state instead of an empty success.
type NoFinancialsError struct {
	CmpyID string
	State  string // e.g. "no-financials-published"
}

func (e *NoFinancialsError) Error() string {
	return fmt.Sprintf("pse-edge companyPage/financial_reports_view.do cmpy_id=%s: %s — the company has no financial statements on Edge; nothing to report", e.CmpyID, e.State)
}

// FinancialRow is one rowheader line: printed strings plus parsed floats.
// Current/Prior are the two value columns (Annual: Current Year / Previous
// Year; Quarterly: Period Ended / Fiscal Year Ended(Audited)).
type FinancialRow struct {
	Item       string   `json:"item"`
	Current    *float64 `json:"current"`
	Prior      *float64 `json:"prior"`
	CurrentRaw string   `json:"current_raw"`
	PriorRaw   string   `json:"prior_raw"`
}

// FinancialSection is one of the Annual/Quarterly blocks.
type FinancialSection struct {
	Period string         `json:"period"` // as printed, e.g. "Dec 31, 2025"
	Units  string         `json:"units,omitempty"`
	Rows   []FinancialRow `json:"rows"`
}

// FinancialReport is the parsed financial_reports_view.do page.
type FinancialReport struct {
	Annual    *FinancialSection `json:"annual"`
	Quarterly *FinancialSection `json:"quarterly"`
}

var (
	finSectionHeadRE = regexp.MustCompile(`<h3>\s*(Annual|Quarterly)\s*</h3>`)
	finPeriodRE      = regexp.MustCompile(`For the (?:fiscal year|period) ended\s*:\s*([^<]+)`)
	finUnitsRE       = regexp.MustCompile(`Currency\(and units, if applicable\)\s*:\s*([^<]+)`)
	finRowRE         = regexp.MustCompile(`(?s)<th>([^<]+)</th>\s*<td class="alignR">([^<]*)</td>\s*<td class="alignR">([^<]*)</td>`)
	parenRE          = regexp.MustCompile(`^\((.*)\)$`)
)

// parseFinancialValue parses one printed cell: comma-grouped decimal,
// parenthesized = negative, blank/dash = nil.
func parseFinancialValue(raw string) *float64 {
	s := cleanText(raw)
	neg := false
	if m := parenRE.FindStringSubmatch(s); m != nil {
		neg = true
		s = m[1]
	}
	f := parseFloatLoose(s)
	if f == nil {
		return nil
	}
	if neg {
		v := -*f
		return &v
	}
	return f
}

// ParseFinancialReports parses a financial_reports_view.do page into typed
// Annual/Quarterly sections. Challenge pages and shells (no company header)
// are typed hard errors; a rendered page with zero data rows is a typed
// NoFinancialsError ("no-financials-published"), never an empty success.
// cmpyID is used only for error messages.
func ParseFinancialReports(htmlBody, cmpyID string) (*FinancialReport, error) {
	if err := detectChallenge("companyPage/financial_reports_view.do", htmlBody); err != nil {
		return nil, err
	}
	if !strings.Contains(htmlBody, `class="compInfo"`) {
		return nil, &ShellPageError{Endpoint: "companyPage/financial_reports_view.do", Reason: "company header absent (bogus cmpy_id?)"}
	}

	heads := finSectionHeadRE.FindAllStringSubmatchIndex(htmlBody, -1)
	report := &FinancialReport{}
	for i, h := range heads {
		name := htmlBody[h[2]:h[3]]
		start := h[1]
		end := len(htmlBody)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		section := parseFinancialSection(htmlBody[start:end])
		switch name {
		case "Annual":
			report.Annual = section
		case "Quarterly":
			report.Quarterly = section
		}
	}

	rows := 0
	if report.Annual != nil {
		rows += len(report.Annual.Rows)
	}
	if report.Quarterly != nil {
		rows += len(report.Quarterly.Rows)
	}
	if rows == 0 {
		return nil, &NoFinancialsError{CmpyID: cmpyID, State: "no-financials-published"}
	}
	return report, nil
}

func parseFinancialSection(fragment string) *FinancialSection {
	s := &FinancialSection{Rows: make([]FinancialRow, 0, 16)}
	if m := finPeriodRE.FindStringSubmatch(fragment); m != nil {
		s.Period = cleanText(m[1])
	}
	if m := finUnitsRE.FindStringSubmatch(fragment); m != nil {
		s.Units = cleanText(m[1])
	}
	for _, m := range finRowRE.FindAllStringSubmatch(fragment, -1) {
		s.Rows = append(s.Rows, FinancialRow{
			Item:       cleanText(m[1]),
			Current:    parseFinancialValue(m[2]),
			Prior:      parseFinancialValue(m[3]),
			CurrentRaw: cleanText(m[2]),
			PriorRaw:   cleanText(m[3]),
		})
	}
	return s
}
