// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
	"github.com/ph-commons/pse-edge-pp-cli/internal/quotationreport"
)

func TestQuotationReportReadsRetainedEvidence(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	data := filepath.Join(home, "data")
	close := 19.98
	pdf := quotationreport.PDF{URL: "https://documents.pse.com.ph/kept.pdf", Body: []byte("%PDF-kept"), SHA: "abc123"}
	parsed := &quotationreport.Parsed{
		Rows: []quotationreport.Row{{
			Symbol: "AT", IssueName: "ATLAS MINING", Close: &close, Currency: "PHP",
			SessionDate: "2026-09-24", FieldStatus: map[string]string{"close": "ok"},
		}, {
			Symbol: "DHI", IssueName: "DOMINION HLDG", Currency: "PHP",
			SessionDate: "2026-09-24", FieldStatus: map[string]string{"close": "reported_dash"},
		}},
		Ambiguous: []quotationreport.AmbiguousSymbol{},
	}
	if _, _, _, err := quotationreport.Admit(data, "2026-09-24", pdf, quotationreport.Discovery{PDFURL: pdf.URL, SessionTitle: "September 24, 2026"}, parsed, time.Date(2026, 9, 24, 19, 40, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PSE_QUOTATION_LISTING_URL", "http://127.0.0.1:1/market-report/")
	root := RootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"quotation-report", "--date", "20260924", "--symbols", "AT,DHI,NOPE", "--json", "--no-learn", "--home", home})
	err := root.Execute()
	if ExitCode(err) != 3 {
		t.Fatalf("exit=%d err=%v stderr=%s", ExitCode(err), err, errb.String())
	}
	var report quotationreport.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if report.Status != quotationreport.StatusMissingSymbols || report.Contract != quotationreport.ContractID {
		t.Fatalf("report status=%s contract=%s", report.Status, report.Contract)
	}
	if !report.Reused || report.Source == nil || report.Source.SHA256 != "abc123" {
		t.Fatalf("source=%+v reused=%v", report.Source, report.Reused)
	}
	if len(report.Rows) != 2 || report.Rows[0].Close == nil || *report.Rows[0].Close != 19.98 {
		t.Fatalf("rows=%+v", report.Rows)
	}
	if report.Coverage.PresentNullClose[0] != "DHI" || report.Coverage.Absent[0] != "NOPE" {
		t.Fatalf("coverage=%+v", report.Coverage)
	}
}

func TestQuotationReportHelpMentionsLimits(t *testing.T) {
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"quotation-report", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"pse-edge-quotation-report-v1", "pse-edge-export-eod-v1", "reported_dash", "16:30", "pdftotext"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("help missing %q\n%s", want, help)
		}
	}
}
