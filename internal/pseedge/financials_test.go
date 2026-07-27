// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"errors"
	"os"
	"testing"
)

// gtcapFinancialsFixture is the live financial_reports_view.do?cmpy_id=633
// page captured 2026-07-27 (Annual FY Dec 31, 2025; Quarterly Mar 31, 2026).
func gtcapFinancialsFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/financial_reports_gtcap.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(data)
}

func findFinancialRow(rows []FinancialRow, item string) *FinancialRow {
	for i := range rows {
		if rows[i].Item == item {
			return &rows[i]
		}
	}
	return nil
}

func TestParseFinancialReportsGTCAP(t *testing.T) {
	report, err := ParseFinancialReports(gtcapFinancialsFixture(t), "633")
	if err != nil {
		t.Fatalf("ParseFinancialReports: %v", err)
	}
	if report.Annual == nil || report.Quarterly == nil {
		t.Fatalf("expected both Annual and Quarterly sections, got annual=%v quarterly=%v", report.Annual, report.Quarterly)
	}
	if report.Annual.Period != "Dec 31, 2025" {
		t.Errorf("annual period = %q, want %q", report.Annual.Period, "Dec 31, 2025")
	}
	if report.Quarterly.Period != "Mar 31, 2026" {
		t.Errorf("quarterly period = %q, want %q", report.Quarterly.Period, "Mar 31, 2026")
	}
	if report.Annual.Units != "In million pesos" {
		t.Errorf("annual units = %q, want %q", report.Annual.Units, "In million pesos")
	}

	// Table-driven expected values from the captured Annual section.
	annualWant := []struct {
		item                 string
		current, prior       float64
		currentRaw, priorRaw string
	}{
		{"Current Assets", 159421, 148381, "159,421", "148,381"},
		{"Total Assets", 519043, 474088, "519,043", "474,088"},
		{"Current Liabilities", 117849, 107127, "117,849", "107,127"},
		{"Total Liabilities", 201710, 194238, "201,710", "194,238"},
		{"Retained Earnings/(Deficit)", 193321, 161734, "193,321", "161,734"},
		{"Stockholders' Equity", 317333, 279850, "317,333", "279,850"},
		{"Stockholders' Equity - Parent", 298678, 262517, "298,678", "262,517"},
		{"Book Value Per Share", 1354.29, 1186.32, "1,354.29", "1,186.32"},
		{"Gross Revenue", 346901, 321527, "346,901", "321,527"},
		{"Gross Expense", 297972, 277999, "297,972", "277,999"},
		{"Net Income/(Loss) After Tax", 43084, 37518, "43,084", "37,518"},
		{"Earnings/(Loss) Per Share (Basic)", 154.72, 132.00, "154.72", "132.00"},
	}
	for _, w := range annualWant {
		row := findFinancialRow(report.Annual.Rows, w.item)
		if row == nil {
			t.Errorf("annual row %q missing", w.item)
			continue
		}
		if row.Current == nil || *row.Current != w.current {
			t.Errorf("annual %q current = %v, want %v", w.item, row.Current, w.current)
		}
		if row.Prior == nil || *row.Prior != w.prior {
			t.Errorf("annual %q prior = %v, want %v", w.item, row.Prior, w.prior)
		}
		if row.CurrentRaw != w.currentRaw || row.PriorRaw != w.priorRaw {
			t.Errorf("annual %q raw = (%q,%q), want (%q,%q)", w.item, row.CurrentRaw, row.PriorRaw, w.currentRaw, w.priorRaw)
		}
	}

	// Quarterly must be present and carry values distinct from Annual.
	qta := findFinancialRow(report.Quarterly.Rows, "Total Assets")
	if qta == nil || qta.Current == nil {
		t.Fatalf("quarterly Total Assets missing")
	}
	ata := findFinancialRow(report.Annual.Rows, "Total Assets")
	if *qta.Current == *ata.Current {
		t.Errorf("quarterly Total Assets current (%v) should differ from annual current (%v)", *qta.Current, *ata.Current)
	}
}

func TestParseFinancialValueParenNegative(t *testing.T) {
	cases := []struct {
		raw  string
		want *float64
	}{
		{"(1,234.5)", fptr(-1234.5)},
		{"1,234.5", fptr(1234.5)},
		{"", nil},
		{"-", nil},
	}
	for _, c := range cases {
		got := parseFinancialValue(c.raw)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("parseFinancialValue(%q) = %v, want nil", c.raw, *got)
		case c.want != nil && (got == nil || *got != *c.want):
			t.Errorf("parseFinancialValue(%q) = %v, want %v", c.raw, got, *c.want)
		}
	}
}

func fptr(f float64) *float64 { return &f }

func TestParseFinancialReportsNoTables(t *testing.T) {
	page := `<html><body><div class="compInfo"><p>Shell Co.</p></div>
<p>Information in this page will become available upon submission of the Company of its latest financial statements.</p>
</body></html>`
	_, err := ParseFinancialReports(page, "999")
	var nfe *NoFinancialsError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NoFinancialsError, got %v", err)
	}
	if nfe.State != "no-financials-published" {
		t.Errorf("state = %q, want no-financials-published", nfe.State)
	}
}

func TestParseFinancialReportsShell(t *testing.T) {
	_, err := ParseFinancialReports("<html><body>nothing here</body></html>", "999")
	var shell *ShellPageError
	if !errors.As(err, &shell) {
		t.Fatalf("expected ShellPageError, got %v", err)
	}
}

func TestParseFinancialReportsChallenge(t *testing.T) {
	_, err := ParseFinancialReports("<html><title>Just a moment</title></html>", "633")
	var ch *ChallengeError
	if !errors.As(err, &ch) {
		t.Fatalf("expected ChallengeError, got %v", err)
	}
}
