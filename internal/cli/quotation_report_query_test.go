// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
	"github.com/ph-commons/pse-edge-pp-cli/internal/quotationreport"
)

func TestQuotationReportQueryRange(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	data := filepath.Join(home, "data")
	closeV := 19.98
	volume := 150000.0
	net := -12345.5
	admitSession(t, data, "2026-09-24", "abc", []quotationreport.Row{{
		Symbol: "AT", IssueName: "ATLAS", Close: &closeV, Volume: &volume, NetForeign: &net,
		FieldStatus: map[string]string{"close": "ok", "volume": "ok", "net_foreign": "ok"},
	}}, []quotationreport.AmbiguousSymbol{{Symbol: "DUP", Locators: []string{"p1", "p2"}}})
	admitSession(t, data, "2026-09-25", "def", []quotationreport.Row{{
		Symbol: "DHI", IssueName: "DOMINION", FieldStatus: map[string]string{"close": "reported_dash"},
	}}, nil)
	out := runQuotation(t, "quotation-report", "query", "--from", "20260924", "--to", "2026-09-25", "--symbols", "AT,DHI,NOPE", "--json", "--no-learn", "--home", home)
	if bytes.Contains(out, []byte(`"close": 0`)) || bytes.Contains(out, []byte(`"close":0`)) {
		t.Fatalf("dash encoded as zero\n%s", out)
	}
	if !bytes.Contains(out, []byte(`"close": null`)) {
		t.Fatalf("missing null close\n%s", out)
	}
	var payload quotationRowsPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.Contract != quotationRowsContract || payload.Status != "ok" || payload.Selected.Mode != "current" || payload.Selected.SHA256 != "" {
		t.Fatalf("header=%+v", payload)
	}
	if len(payload.Sessions) != 2 {
		t.Fatalf("sessions=%d", len(payload.Sessions))
	}
	if payload.Sessions[0].Revision.SHA256 != "abc" || !payload.Sessions[0].Revision.Current {
		t.Fatalf("first=%+v", payload.Sessions[0].Revision)
	}
	if payload.Sessions[1].Revision.SHA256 != "def" || !payload.Sessions[1].Revision.Current {
		t.Fatalf("second=%+v", payload.Sessions[1].Revision)
	}
	if len(payload.Sessions[0].Rows) != 1 || payload.Sessions[0].Rows[0].Symbol != "AT" || payload.Sessions[0].Rows[0].Close == nil || *payload.Sessions[0].Rows[0].Close != 19.98 {
		t.Fatalf("at=%+v", payload.Sessions[0].Rows)
	}
	if payload.Sessions[0].Rows[0].Volume == nil || *payload.Sessions[0].Rows[0].Volume != 150000 || payload.Sessions[0].Rows[0].NetForeign == nil || *payload.Sessions[0].Rows[0].NetForeign != -12345.5 {
		t.Fatalf("measures=%+v", payload.Sessions[0].Rows[0])
	}
	if payload.Sessions[0].Rows[0].FieldStatus["close"] != "ok" {
		t.Fatalf("status=%v", payload.Sessions[0].Rows[0].FieldStatus)
	}
	if !stringListHas(payload.Sessions[0].Coverage.Absent, "NOPE") || !stringListHas(payload.Sessions[1].Coverage.Absent, "NOPE") {
		t.Fatalf("absent=%v %v", payload.Sessions[0].Coverage.Absent, payload.Sessions[1].Coverage.Absent)
	}
	if payload.Sessions[1].Rows[0].Close != nil || payload.Sessions[1].Rows[0].FieldStatus["close"] != "reported_dash" {
		t.Fatalf("dash=%+v", payload.Sessions[1].Rows)
	}
	if !stringListHas(payload.Sessions[1].Coverage.PresentNullClose, "DHI") {
		t.Fatalf("null close=%v", payload.Sessions[1].Coverage.PresentNullClose)
	}
	for _, session := range payload.Sessions {
		for _, row := range session.Rows {
			if row.Symbol == "DUP" || row.Symbol == "NOPE" {
				t.Fatalf("invented row %s", row.Symbol)
			}
		}
	}
	if len(payload.Sessions[0].Ambiguous) != 1 || payload.Sessions[0].Ambiguous[0].Symbol != "DUP" {
		t.Fatalf("ambiguous=%+v", payload.Sessions[0].Ambiguous)
	}
	if !stringListHas(payload.Sessions[0].Coverage.Ambiguous, "DUP") {
		t.Fatalf("coverage ambiguous=%v", payload.Sessions[0].Coverage.Ambiguous)
	}
}

func TestQuotationReportQueryPreviousSHA(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	data := filepath.Join(home, "data")
	first := 1.0
	second := 9.0
	admitSession(t, data, "2026-09-24", "aaa", []quotationreport.Row{{Symbol: "AT", Close: &first, FieldStatus: map[string]string{"close": "ok"}}}, nil)
	admitSession(t, data, "2026-09-24", "bbb", []quotationreport.Row{{Symbol: "AT", Close: &second, FieldStatus: map[string]string{"close": "ok"}}}, nil)
	out := runQuotation(t, "quotation-report", "query", "--date", "20260924", "--sha", "aaa", "--json", "--no-learn", "--home", home)
	var payload quotationRowsPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.Selected.Mode != "sha" || payload.Selected.SHA256 != "aaa" || len(payload.Sessions) != 1 {
		t.Fatalf("selected=%+v sessions=%d", payload.Selected, len(payload.Sessions))
	}
	session := payload.Sessions[0]
	if session.Revision.Current || session.Revision.SHA256 != "aaa" || len(session.Rows) != 1 || session.Rows[0].SourceSHA256 != "aaa" || session.Rows[0].Close == nil || *session.Rows[0].Close != 1 {
		t.Fatalf("session=%+v rows=%+v", session.Revision, session.Rows)
	}
}

func TestQuotationReportIndexRebuildsSQLite(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	data := filepath.Join(home, "data")
	closeV := 19.98
	admitSession(t, data, "2026-09-24", "abc", []quotationreport.Row{{
		Symbol: "AT", Close: &closeV, FieldStatus: map[string]string{"close": "ok"},
	}}, nil)
	dbPath := filepath.Join(data, "data.db")
	for _, suf := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(dbPath + suf); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(data, "quotation-reports", "2026-09-24", "last-error.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexOut := runQuotation(t, "quotation-report", "index", "--json", "--no-learn", "--home", home)
	var indexed quotationIndexPayload
	if err := json.Unmarshal(indexOut, &indexed); err != nil {
		t.Fatalf("index json: %v\n%s", err, indexOut)
	}
	if indexed.Contract != quotationIndexContract || len(indexed.Indexed) != 1 || indexed.Indexed[0].SHA256 != "abc" {
		t.Fatalf("indexed=%+v", indexed)
	}
	foundSkip := false
	for _, skip := range indexed.Skipped {
		if skip.Path == "quotation-reports/2026-09-24/last-error.json" && skip.Reason == "not_listed" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("skipped=%+v", indexed.Skipped)
	}
	out := runQuotation(t, "quotation-report", "query", "--date", "20260924", "--json", "--no-learn", "--home", home)
	var payload quotationRowsPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("query json: %v\n%s", err, out)
	}
	if len(payload.Sessions) != 1 || payload.Sessions[0].Rows[0].Close == nil || *payload.Sessions[0].Rows[0].Close != 19.98 {
		t.Fatalf("query=%+v", payload.Sessions)
	}
}

func TestQuotationReportIndexDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	if err := os.MkdirAll(filepath.Join(home, "data", "quotation-reports", "2026-09-24"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"quotation-report", "index", "--date", "20260924", "--dry-run", "--no-learn", "--home", home})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("dry-run quotation-report index session=2026-09-24")) {
		t.Fatalf("dry-run output=%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "data", "data.db")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote sqlite: %v", err)
	}
}

func TestQuotationReportQueryEmptyIsReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	out := runQuotation(t, "quotation-report", "query", "--date", "20260924", "--json", "--no-learn", "--home", home)
	var payload quotationRowsPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.Status != "ok" || len(payload.Sessions) != 0 {
		t.Fatalf("payload=%+v", payload)
	}
	if _, err := os.Stat(filepath.Join(home, "data", "data.db")); !os.IsNotExist(err) {
		t.Fatalf("query wrote sqlite: %v", err)
	}
}

func TestQuotationReportQueryUnknownSHA(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	closeV := 1.0
	admitSession(t, filepath.Join(home, "data"), "2026-09-24", "aaa", []quotationreport.Row{{Symbol: "AT", Close: &closeV}}, nil)
	root := RootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"quotation-report", "query", "--date", "20260924", "--sha", "missing", "--json", "--no-learn", "--home", home})
	err := root.Execute()
	if ExitCode(err) != 3 {
		t.Fatalf("exit=%d err=%v stderr=%s", ExitCode(err), err, errb.String())
	}
}

func TestQuotationReportQueryRejectsMixedDateFlags(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"quotation-report", "query", "--date", "20260924", "--from", "20260924", "--no-learn", "--home", home})
	err := root.Execute()
	if ExitCode(err) != 2 {
		t.Fatalf("exit=%d err=%v out=%s", ExitCode(err), err, out.String())
	}
	root = RootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"quotation-report", "query", "--from", "20260924", "--no-learn", "--home", home})
	err = root.Execute()
	if ExitCode(err) != 2 {
		t.Fatalf("from-only exit=%d err=%v out=%s", ExitCode(err), err, out.String())
	}
}

func admitSession(t *testing.T, data, session, sha string, rows []quotationreport.Row, amb []quotationreport.AmbiguousSymbol) {
	t.Helper()
	for i := range rows {
		rows[i].SessionDate = session
		rows[i].SourceSHA256 = sha
		rows[i].SourceType = quotationreport.SourceType
		if rows[i].Page == 0 {
			rows[i].Page = 1
		}
		if rows[i].RowLocator == "" {
			rows[i].RowLocator = "r1"
		}
		rows[i].FieldStatus = fillQuoteStatus(rows[i])
	}
	if amb == nil {
		amb = []quotationreport.AmbiguousSymbol{}
	}
	pdf := quotationreport.PDF{URL: "https://documents.pse.com.ph/" + sha + ".pdf", Body: []byte("%PDF-" + sha), SHA: sha}
	parsed := &quotationreport.Parsed{SessionDate: session, Rows: rows, Ambiguous: amb}
	disc := quotationreport.Discovery{
		ListingURL: "https://www.pse.com.ph/market-report/", PDFURL: pdf.URL,
		MatchedTitle: "End of Day Quotes", MatchedSlug: "end-of-day-quotes",
		UploadDate: "September 24, 2026", SessionTitle: session,
	}
	if _, _, _, err := quotationreport.Admit(data, session, pdf, disc, parsed, time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

func fillQuoteStatus(row quotationreport.Row) map[string]string {
	out := map[string]string{}
	for k, v := range row.FieldStatus {
		out[k] = v
	}
	set := func(name string, ptr *float64) {
		if _, ok := out[name]; ok {
			return
		}
		if ptr == nil {
			out[name] = quotationreport.FieldReportedDash
		} else {
			out[name] = quotationreport.FieldOK
		}
	}
	set("bid", row.Bid)
	set("ask", row.Ask)
	set("open", row.Open)
	set("high", row.High)
	set("low", row.Low)
	set("close", row.Close)
	set("volume", row.Volume)
	set("value", row.Value)
	set("net_foreign", row.NetForeign)
	return out
}

func runQuotation(t *testing.T, args ...string) []byte {
	t.Helper()
	root := RootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("args=%v err=%v stderr=%s stdout=%s", args, err, errb.String(), out.String())
	}
	return out.Bytes()
}

func stringListHas(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
