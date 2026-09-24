// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseSyntheticCases(t *testing.T) {
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	header := "Issue Name Symbol Bid Ask Open High Low Close Volume Value, PHP NetForeign"
	text := strings.Join([]string{
		"The Philippine Stock Exchange, Inc.",
		"Daily Quotation Report",
		"September 24, 2026",
		"MAIN BOARD",
		header,
		"BDO UNIBANK BDO 111.3 111.5 113.5 114 110.1 111.3 2,146,520 240,105,096 (65,122,622)",
		"DOMINION HLDG DHI - - - - - - - - -",
		"FINANCIALS SECTOR TOTAL 55,300,030 724,870,328",
		"SM 503.00 240,000 120,720,000.00",
		"FOO ONE AAA 1 2 3 4 5 6 7 8 -",
		"FOO TWO AAA 1 2 3 4 5 6 9 8 -",
		"\f",
		"The Philippine Stock Exchange, Inc.",
		"Daily Quotation Report",
		"September 24, 2026",
		header,
		"MERALCO MER 446.4 447 463.2 466.4 445 447 322,680 144,774,416 12,835,624",
		"GRAND TOTAL 1 2",
	}, "\n")
	parsed, err := ParseLayout(text, session, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Ambiguous) != 1 || parsed.Ambiguous[0].Symbol != "AAA" {
		t.Fatalf("ambiguous=%+v", parsed.Ambiguous)
	}
	by := map[string]Row{}
	for _, row := range parsed.Rows {
		by[row.Symbol] = row
		if row.Symbol == "TOTAL" || strings.Contains(row.IssueName, "TOTAL") {
			t.Fatalf("total admitted: %+v", row)
		}
	}
	if _, ok := by["AAA"]; ok {
		t.Fatal("ambiguous symbol was admitted")
	}
	if _, ok := by["SM"]; ok {
		t.Fatal("block sale was admitted")
	}
	bdo := by["BDO"]
	if bdo.IssueName != "BDO UNIBANK" || bdo.NetForeign == nil || *bdo.NetForeign != -65122622 {
		t.Fatalf("bdo=%+v", bdo)
	}
	dhi := by["DHI"]
	if dhi.Close != nil || dhi.FieldStatus["close"] != FieldReportedDash {
		t.Fatalf("dhi=%+v", dhi)
	}
	if dhi.Volume != nil {
		t.Fatal("dash volume became a number")
	}
	mer := by["MER"]
	if mer.Page != 2 || mer.Close == nil || *mer.Close != 447 {
		t.Fatalf("mer=%+v", mer)
	}
}

func TestParseRejectsWrongDateAndGarbage(t *testing.T) {
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	wrong := "Daily Quotation Report\nSeptember 23, 2026\nMAIN BOARD\nBDO UNIBANK BDO 1 1 1 1 1 1 1 1 1\n"
	if _, err := ParseLayout(wrong, session, "x"); err == nil || statusOf(err) != StatusWrongSessionDate {
		t.Fatalf("wrong date err=%v", err)
	}
	if _, err := ParseLayout("not a report\n", session, "x"); err == nil || statusOf(err) != StatusMalformedDocument {
		t.Fatalf("garbage err=%v", err)
	}
	empty := "Daily Quotation Report\nSeptember 24, 2026\nMAIN BOARD\nGRAND TOTAL 1 2\n"
	if _, err := ParseLayout(empty, session, "x"); err == nil || statusOf(err) != StatusPartialParse {
		t.Fatalf("partial err=%v", err)
	}
}

func TestParseRejectsPartialAndBadColumns(t *testing.T) {
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	header := "Issue Name Symbol Bid Ask Open High Low Close Volume Value, PHP NetForeign"
	base := strings.Join([]string{
		"Daily Quotation Report",
		"September 24, 2026",
		header,
		"BDO UNIBANK BDO 1 2 3 4 5 6 7 8 9",
	}, "\n")
	if _, err := ParseLayout(base, session, "x"); err == nil || statusOf(err) != StatusPartialParse {
		t.Fatalf("truncated err=%v", err)
	}
	broken := base + "\nALPHA AAA 1 2 3 4 5 BROKEN 7 8 9\nGRAND TOTAL 1 2\n"
	if _, err := ParseLayout(broken, session, "x"); err == nil || statusOf(err) != StatusPartialParse || !strings.Contains(err.Error(), "malformed security row") {
		t.Fatalf("broken err=%v", err)
	}
	missing := strings.Join([]string{
		"Daily Quotation Report",
		"September 24, 2026",
		"ALPHA AAA 1 2 3 4 5 6 7 8 9",
		"GRAND TOTAL 1 2",
	}, "\n")
	if _, err := ParseLayout(missing, session, "x"); err == nil || statusOf(err) != StatusMalformedDocument {
		t.Fatalf("missing header err=%v", err)
	}
	reordered := strings.Join([]string{
		"Daily Quotation Report",
		"September 24, 2026",
		"Issue Symbol Bid Ask Open High Low Volume Close Value NetForeign",
		"ALPHA AAA 1 2 3 4 5 600 7 8 9",
		"GRAND TOTAL 1 2",
	}, "\n")
	if _, err := ParseLayout(reordered, session, "x"); err == nil || statusOf(err) != StatusMalformedDocument {
		t.Fatalf("reordered err=%v", err)
	}
}

func TestSeptember24Fixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/20260924-eod.pdf")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	const wantSHA = "aa24a2f55058ef8b4dc573091471971351a8c821ba5e9c9698a1afb2874754a0"
	if hex.EncodeToString(sum[:]) != wantSHA {
		t.Fatalf("pdf sha=%s", hex.EncodeToString(sum[:]))
	}
	layout, err := os.ReadFile("testdata/20260924-eod.layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	if path, lookErr := exec.LookPath("pdftotext"); lookErr == nil {
		cmd := exec.Command(path, "-layout", "-enc", "UTF-8", "testdata/20260924-eod.pdf", "-")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, layout) {
			t.Fatalf("pdftotext output drifted from the retained layout fixture (%d vs %d)", len(out), len(layout))
		}
	}
	session := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	parsed, err := ParseLayout(string(layout), session, wantSHA)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Ambiguous) != 0 {
		t.Fatalf("ambiguous=%+v", parsed.Ambiguous)
	}
	if len(parsed.Rows) != 383 {
		t.Fatalf("rows=%d", len(parsed.Rows))
	}
	by := map[string]Row{}
	for _, row := range parsed.Rows {
		by[row.Symbol] = row
	}
	checks := map[string]float64{"AT": 19.98, "APX": 17.64, "FGEN": 19.38, "MER": 447}
	for sym, close := range checks {
		row, ok := by[sym]
		if !ok || row.Close == nil || math.Abs(*row.Close-close) > 1e-6 {
			t.Fatalf("%s row=%+v", sym, row)
		}
		if row.SourceSHA256 != wantSHA || row.SessionDate != "2026-09-24" || row.Currency != "PHP" || row.IssueName == "" || row.IssueName == "net_foreign" {
			t.Fatalf("%s provenance=%+v", sym, row)
		}
	}
	if by["AT"].IssueName != "ATLAS MINING" {
		t.Fatalf("AT name=%q", by["AT"].IssueName)
	}
	if by["MER"].Page < 2 {
		t.Fatalf("MER page=%d", by["MER"].Page)
	}
	dhi, ok := by["DHI"]
	if !ok || dhi.Close != nil || dhi.FieldStatus["close"] != FieldReportedDash {
		t.Fatalf("DHI=%+v", dhi)
	}
	if by["BDO"].NetForeign == nil || *by["BDO"].NetForeign >= 0 {
		t.Fatalf("BDO net=%v", by["BDO"].NetForeign)
	}
	dds := by["DMPA1"]
	if dds.Currency != "USD" || dds.Close != nil || dds.Board != "DDS" {
		t.Fatalf("DMPA1=%+v", dds)
	}
	cov, rows, status := Cover(parsed, []string{"AT", "DHI", "NOPE"})
	if status != StatusMissingSymbols || len(rows) != 2 || cov.Absent[0] != "NOPE" {
		t.Fatalf("cover status=%s rows=%d cov=%+v", status, len(rows), cov)
	}
	if len(cov.PresentNullClose) != 1 || cov.PresentNullClose[0] != "DHI" {
		t.Fatalf("null close=%v", cov.PresentNullClose)
	}
}

func TestRejectsMissingFixtureMeasure(t *testing.T) {
	layout, err := os.ReadFile("testdata/20260924-eod.layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(layout), "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == "MERALCO" && fields[1] == "MER" {
			// Remove the close without changing any other part of the complete report.
			lines[i] = strings.Join(append(fields[:7], fields[8:]...), " ")
			_, err := ParseLayout(strings.Join(lines, "\n"), time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "x")
			if err == nil || statusOf(err) != StatusPartialParse {
				t.Fatalf("missing MER close: %v", err)
			}
			return
		}
	}
	t.Fatal("MER fixture row not found")
}

func TestRejectsIncompleteOrReorderedFullSchema(t *testing.T) {
	for _, header := range []string{
		"Issue Symbol Bid Ask Open High Low Value Close Volume NetForeign",
		"Issue Symbol Bid Ask Open High Low Close Volume NetForeign Value",
		"Issue Symbol Bid Ask Open High Low Close Volume Value",
		"Issue Symbol Bid Ask Open High Low Close Volume\nValue NetForeign",
	} {
		t.Run(header, func(t *testing.T) {
			text := "Daily Quotation Report\nSeptember 24, 2026\n" + header + "\nALPHA AAA 1 2 3 4 5 800 6 700 9\nGRAND TOTAL 1 2"
			_, err := ParseLayout(text, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "x")
			if err == nil || statusOf(err) != StatusMalformedDocument {
				t.Fatalf("unsupported columns: %v", err)
			}
		})
	}
}

func TestRejectsReorderedMultilineFixtureHeader(t *testing.T) {
	layout, err := os.ReadFile("testdata/20260924-eod.layout.txt")
	if err != nil {
		t.Fatal(err)
	}
	original := "Volume               Value, USD"
	if !strings.Contains(string(layout), original) {
		t.Fatal("DDS header not found")
	}
	text := strings.Replace(string(layout), original, "Value, USD           Volume", 1)
	_, err = ParseLayout(text, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "x")
	if err == nil || statusOf(err) != StatusMalformedDocument {
		t.Fatalf("reordered multiline DDS header: %v", err)
	}
}
