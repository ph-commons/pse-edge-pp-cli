// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Fixtures are trimmed fragments of live pages fetched 2026-07-27 from
// edge.pse.com.ph / frames.pse.com.ph (public endpoints, no auth).

const directoryFixture = `<span class="count">

[1 /
6]
[Total 282]
</span>
<table class="list">
<thead>
  <tr>
    <th>Company Name</th>
    <th>Stock Symbol</th>
  </tr>
</thead>
<tbody>
    <tr>
      <td><a href="#company" onclick="cmDetail('55','347');return false;">Asia Amalgamated Holdings Corporation</a></td>
      <td class="alignC"><a href="#company" onclick="cmDetail('55','347');return false;">AAA</a></td>
      <td>Holding Firms</td>
    </tr>
    <tr>
      <td><a href="#company" onclick="cmDetail('19','181');return false;">Atok-Big Wedge Co., Inc.</a></td>
      <td class="alignC"><a href="#company" onclick="cmDetail('19','181');return false;">AB</a></td>
      <td>Mining and Oil</td>
    </tr>
    <tr>
      <td><a href="#company" onclick="cmDetail('633','628');return false;">GT Capital Holdings, Inc.</a></td>
      <td class="alignC"><a href="#company" onclick="cmDetail('633','628');return false;">GTCAP</a></td>
      <td>Holding Firms</td>
    </tr>
</tbody>
</table>`

const directoryEmptyLaterPageFixture = `<span class="count">[7 / 6] [Total 282]</span>
<table class="list">
<tbody>
</tbody>
</table>`

const challengeFixture = `<!DOCTYPE html><html><head><title>Just a moment...</title>
<script src="/cdn-cgi/challenge-platform/orchestrate/chl_page/v1"></script></head>
<body><div id="cf-chl-widget"></div></body></html>`

// shellStockFixture mirrors the live ~1KB body served for a bogus cmpy_id.
const shellStockFixture = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html><head><title>Message</title></head>
<body>
<form name="baseForm">
<textarea name="message" style="display:none;">Stock symbol not found.</textarea>
</form>
</body></html>`

const historyFixture = `{"chartData":[
{"OPEN":8.3,"VALUE":1.1852847E7,"CLOSE":8.08,"CHART_DATE":"Jul 13, 2026 00:00:00","HIGH":8.3,"LOW":7.98},
{"OPEN":8.1,"VALUE":9651781.0,"CLOSE":8.29,"CHART_DATE":"Jul 14, 2026 00:00:00","HIGH":8.38,"LOW":8.1},
{"OPEN":8.44,"VALUE":3.7152868E7,"CLOSE":8.7,"CHART_DATE":"Jul 16, 2026 00:00:00","HIGH":8.8,"LOW":8.4}]}`

const stockFixture = `<script>
function getDiscData(){
	var sendData = {};
	sendData.cmpy_id = "34";
	sendData.security_id = "320";
}
</script>
<div class="compInfo">
  <p style="">Atlas Consolidated Mining and Development Corporation</p>
</div>
<form name="form1" action="/companyPage/stockData.do">
  <input type="hidden" name="cmpy_id" value="34"/>
  <select name="security_id" onchange="document.form1.submit();">
<option value="320" selected>AT</option>
</select>
  <span style="margin-left:1em;">As of Jul 27, 2026 02:50 PM</span>
</form>
<table class="view">
<tr>
  <th>Last Traded Price</th>
  <td style="text-align:right;padding-right:1.2em;">
12.74</td>
  <th>Open</th>
  <td style="text-align:right;padding-right:1.2em;">
12.30</td>
  <th>Previous Close and Date</th>
  <td style="text-align:right;padding-right:1.2em;">
12.20
    (Jul 24, 2026)
  </td>
</tr>
<tr>
  <th>Change(% Change)</th>
  <td style="text-align:right;padding-right:1.2em;">
up&nbsp;
  0.54
  (4.43%)
  </td>
  <th>High</th>
  <td style="text-align:right;padding-right:1.2em;">
12.90</td>
  <th>P/E Ratio</th>
  <td style="text-align:right;padding-right:1.2em;">
</td>
</tr>
<tr>
  <th>Value</th>
  <td style="text-align:right;padding-right:1.2em;">
    111,851,478.00</td>
  <th>Low</th>
  <td style="text-align:right;padding-right:1.2em;">
12.30</td>
  <th>Sector P/E Ratio</th>
  <td style="text-align:right;padding-right:1.2em;">
</td>
</tr>
<tr>
  <th>Volume</th>
  <td style="text-align:right;padding-right:1.2em;">
    8,760,300</td>
  <th>Average Price</th>
  <td style="text-align:right;padding-right:1.2em;">
12.77</td>
  <th>Book Value</th>
  <td style="text-align:right;padding-right:1.2em;">
</td>
</tr>
<tr>
  <th>52-Week High</th>
  <td style="text-align:right;padding-right:1.2em;">
12.60</td>
  <th>52-Week Low</th>
  <td style="text-align:right;padding-right:1.2em;">
3.49</td>
  <th>P/BV Ratio</th>
  <td style="text-align:right;padding-right:1.2em;">
</td>
</tr>
</table>`

// stockDownFixture exercises the down-prefix sign derivation.
const stockDownFixture = `<script>
sendData.cmpy_id = "34";
sendData.security_id = "320";
</script>
<div class="compInfo"><p>Atlas Consolidated Mining and Development Corporation</p></div>
<option value="320" selected>AT</option>
<table class="view">
<tr>
  <th>Change(% Change)</th>
  <td>
down&nbsp;
  0.54
  (4.43%)
  </td>
</tr>
</table>`

// stockDownLiveNBSPFixture mirrors EDGE stockData.do as observed 2026-07-27
// (issue #8): U+00A0 between "down" and the absolute change, and a leading
// space inside the percent parentheses. Old changeCellRE failed to match
// (phisix-only fallback) or, if only the percent-space were fixed, would
// silently invert the sign by matching from the digits with an empty prefix.
const stockDownLiveNBSPFixture = `<script>
sendData.cmpy_id = "34";
sendData.security_id = "320";
</script>
<div class="compInfo"><p>Atlas Consolidated Mining and Development Corporation</p></div>
<option value="320" selected>AT</option>
<table class="view">
<tr>
  <th>Change(% Change)</th>
  <td style="text-align:right;padding-right:1.2em;">down` + "\u00a0" + ` 0.040 ( 1.32%)</td>
</tr>
</table>`

// stockDownASCIISpacedFixture is the same shape without NBSP — pure ASCII
// "down  0.040 ( 1.32%)" as reported in the issue body.
const stockDownASCIISpacedFixture = `<script>
sendData.cmpy_id = "34";
sendData.security_id = "320";
</script>
<div class="compInfo"><p>Atlas Consolidated Mining and Development Corporation</p></div>
<option value="320" selected>AT</option>
<table class="view">
<tr>
  <th>Change(% Change)</th>
  <td>down  0.040 ( 1.32%)</td>
</tr>
</table>`

// stockClosedFixture has a blank change cell: explicit closed-session
// state, change fields must stay nil (never zero).
const stockClosedFixture = `<script>
sendData.cmpy_id = "34";
sendData.security_id = "320";
</script>
<div class="compInfo"><p>Atlas Consolidated Mining and Development Corporation</p></div>
<option value="320" selected>AT</option>
<table class="view">
<tr>
  <th>Change(% Change)</th>
  <td>
  </td>
  <th>Open</th>
  <td>
</td>
</tr>
</table>`

const compositeFixture = `<div class="dropdown-menu" aria-labelledby="index_dropdown">
<input id="PSEI-key" name="val1" type="hidden" value="{&quot;Id&quot;:&quot;PSEI&quot;,&quot;SectorCode&quot;:&quot;PSEI&quot;,&quot;IndexGroup&quot;:&quot;PSEi INDEX&quot;,&quot;Value&quot;:&quot;6314.9&quot;,&quot;Change&quot;:&quot;33.89&quot;,&quot;PercentChange&quot;:&quot;0.54&quot;,&quot;tradeDate&quot;:&quot;2026-07-27T14:50:00&#x2B;08:00&quot;}" />
<input id="PSEI-value-values" name="val3" type="hidden" value="[[{&quot;time&quot;:1627430400,&quot;high&quot;:6511.83,&quot;open&quot;:6511.83,&quot;low&quot;:6370.25,&quot;close&quot;:6473.03,&quot;volume&quot;:6473},{&quot;time&quot;:1784851200,&quot;high&quot;:6281.01,&quot;open&quot;:6256.35,&quot;low&quot;:6189.23,&quot;close&quot;:6281.01,&quot;volume&quot;:6281}]]" />
<input id="FIN-key" name="val1" type="hidden" value="{&quot;Id&quot;:&quot;FIN&quot;,&quot;SectorCode&quot;:&quot;FIN&quot;,&quot;IndexGroup&quot;:&quot;FINANCIALS INDEX&quot;,&quot;Value&quot;:&quot;1921.89&quot;,&quot;Change&quot;:&quot;14.33&quot;,&quot;PercentChange&quot;:&quot;0.75&quot;,&quot;tradeDate&quot;:&quot;2026-07-27T14:50:00&#x2B;08:00&quot;}" />
<input id="IND-key" name="val1" type="hidden" value="{&quot;Id&quot;:&quot;IND&quot;,&quot;SectorCode&quot;:&quot;IND&quot;,&quot;IndexGroup&quot;:&quot;INDUSTRIAL INDEX&quot;,&quot;Value&quot;:&quot;8443.95&quot;,&quot;Change&quot;:&quot;-3.65&quot;,&quot;PercentChange&quot;:&quot;-0.04&quot;,&quot;tradeDate&quot;:&quot;2026-07-27T14:50:00&#x2B;08:00&quot;}" />
</div>
<table>
<tr>
    <td class="border-0 font-pse historical-body">Total Volume</td>
    <td class="border-0 text-right historical-body">448,077,665</td>
</tr>
<tr>
    <td class="border-0 font-pse historical-body">Total Trades</td>
    <td class="border-0 text-right historical-body">69,416</td>
</tr>
<tr>
    <td class="border-0 font-pse historical-body">Total Value (in PHP)</td>
    <td class="border-0 text-right historical-body">4,813,139,047.36</td>
</tr>
<tr>
    <td class="border-0 font-pse historical-body">Advances</td>
    <td class="border-0 text-right historical-body">94</td>
</tr>
<tr>
    <td class="border-0 font-pse historical-body">Declines</td>
    <td class="border-0 text-right historical-body">75</td>
</tr>
<tr>
    <td class="border-0 font-pse historical-body">Unchanged</td>
    <td class="border-0 text-right historical-body">39</td>
</tr>
</table>`

func fp(t *testing.T, p *float64, name string) float64 {
	t.Helper()
	if p == nil {
		t.Fatalf("%s: expected non-nil value", name)
	}
	return *p
}

func TestParseDirectoryPage(t *testing.T) {
	companies, err := ParseDirectoryPage(directoryFixture, 1)
	if err != nil {
		t.Fatalf("ParseDirectoryPage: %v", err)
	}
	if len(companies) != 3 {
		t.Fatalf("got %d companies, want 3", len(companies))
	}
	want := []Company{
		{CmpyID: 55, SecurityID: 347, Symbol: "AAA", Name: "Asia Amalgamated Holdings Corporation"},
		{CmpyID: 19, SecurityID: 181, Symbol: "AB", Name: "Atok-Big Wedge Co., Inc."},
		{CmpyID: 633, SecurityID: 628, Symbol: "GTCAP", Name: "GT Capital Holdings, Inc."},
	}
	for i, w := range want {
		if companies[i] != w {
			t.Errorf("company[%d] = %+v, want %+v", i, companies[i], w)
		}
	}
}

func TestParseDirectoryPageEmptyLaterPage(t *testing.T) {
	companies, err := ParseDirectoryPage(directoryEmptyLaterPageFixture, 7)
	if err != nil {
		t.Fatalf("empty later page must not error (end-of-listing): %v", err)
	}
	if len(companies) != 0 {
		t.Fatalf("got %d companies, want 0", len(companies))
	}
}

func TestParseDirectoryPageEmptyFirstPageIsError(t *testing.T) {
	_, err := ParseDirectoryPage(directoryEmptyLaterPageFixture, 1)
	var shellErr *ShellPageError
	if !errors.As(err, &shellErr) {
		t.Fatalf("zero rows on page 1 must be *ShellPageError, got %v", err)
	}
}

func TestParseDirectoryPageChallenge(t *testing.T) {
	_, err := ParseDirectoryPage(challengeFixture, 1)
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("challenge page must be *ChallengeError, got %v", err)
	}
}

func TestParseHistoryResponse(t *testing.T) {
	rows, err := ParseHistoryResponse([]byte(historyFixture))
	if err != nil {
		t.Fatalf("ParseHistoryResponse: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	first := rows[0]
	if first.TradingDate != "2026-07-13" {
		t.Errorf("TradingDate = %q, want 2026-07-13", first.TradingDate)
	}
	if first.Close != 8.08 || first.Open != 8.3 || first.High != 8.3 || first.Low != 7.98 {
		t.Errorf("OHLC = %+v", first)
	}
	if first.Value != 1.1852847e7 {
		t.Errorf("Value = %v, want 1.1852847e7 (scientific-notation parse)", first.Value)
	}
	if rows[2].TradingDate != "2026-07-16" {
		t.Errorf("rows[2].TradingDate = %q, want 2026-07-16", rows[2].TradingDate)
	}
}

func TestParseHistoryResponseChallengeHTML(t *testing.T) {
	_, err := ParseHistoryResponse([]byte(challengeFixture))
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("HTML challenge body must be *ChallengeError, got %v", err)
	}
}

func TestValidateEOD(t *testing.T) {
	good := EODRow{TradingDate: "2026-07-13", Open: 8.3, High: 8.3, Low: 7.98, Close: 8.08, Value: 1.1e7}
	if err := ValidateEOD(good); err != nil {
		t.Errorf("valid row rejected: %v", err)
	}
	tests := []struct {
		name string
		row  EODRow
	}{
		{"zero close", EODRow{TradingDate: "2026-07-13", Open: 8, High: 8, Low: 8, Close: 0}},
		{"negative low", EODRow{TradingDate: "2026-07-13", Open: 8, High: 8, Low: -1, Close: 8}},
		{"absurd price", EODRow{TradingDate: "2026-07-13", Open: 8, High: 2e7, Low: 8, Close: 8}},
		{"empty date", EODRow{TradingDate: "", Open: 8, High: 8, Low: 8, Close: 8}},
		{"garbage date", EODRow{TradingDate: "not-a-date", Open: 8, High: 8, Low: 8, Close: 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateEOD(tt.row); err == nil {
				t.Errorf("row %+v should have been rejected", tt.row)
			}
		})
	}
}

func TestParseStockData(t *testing.T) {
	snap, err := ParseStockData(stockFixture)
	if err != nil {
		t.Fatalf("ParseStockData: %v", err)
	}
	if snap.CmpyID != 34 || snap.SecurityID != 320 {
		t.Errorf("ids = %d/%d, want 34/320", snap.CmpyID, snap.SecurityID)
	}
	if snap.Symbol != "AT" {
		t.Errorf("Symbol = %q, want AT", snap.Symbol)
	}
	if snap.Name != "Atlas Consolidated Mining and Development Corporation" {
		t.Errorf("Name = %q", snap.Name)
	}
	if snap.AsOf == "" {
		t.Error("AsOf should be populated")
	}
	if v := fp(t, snap.LastTradedPrice, "LastTradedPrice"); v != 12.74 {
		t.Errorf("LastTradedPrice = %v, want 12.74", v)
	}
	if v := fp(t, snap.Open, "Open"); v != 12.30 {
		t.Errorf("Open = %v, want 12.30", v)
	}
	if v := fp(t, snap.PrevClose, "PrevClose"); v != 12.20 {
		t.Errorf("PrevClose = %v, want 12.20", v)
	}
	if snap.PrevCloseDate != "2026-07-24" {
		t.Errorf("PrevCloseDate = %q, want 2026-07-24", snap.PrevCloseDate)
	}
	if v := fp(t, snap.Change, "Change"); v != 0.54 {
		t.Errorf("Change = %v, want +0.54 (unsigned cell + up prefix)", v)
	}
	if v := fp(t, snap.PctChange, "PctChange"); v != 4.43 {
		t.Errorf("PctChange = %v, want 4.43", v)
	}
	if v := fp(t, snap.Value, "Value"); v != 111851478.00 {
		t.Errorf("Value = %v (comma-grouped parse)", v)
	}
	if v := fp(t, snap.Volume, "Volume"); v != 8760300 {
		t.Errorf("Volume = %v", v)
	}
	if v := fp(t, snap.Week52Low, "Week52Low"); v != 3.49 {
		t.Errorf("Week52Low = %v", v)
	}
}

func TestParseStockDataDownSign(t *testing.T) {
	snap, err := ParseStockData(stockDownFixture)
	if err != nil {
		t.Fatalf("ParseStockData: %v", err)
	}
	if v := fp(t, snap.Change, "Change"); v != -0.54 {
		t.Errorf("Change = %v, want -0.54 (down prefix derives negative sign)", v)
	}
	if v := fp(t, snap.PctChange, "PctChange"); v != -4.43 {
		t.Errorf("PctChange = %v, want -4.43", v)
	}
}

// TestParseStockDataDownDayLiveMarkup covers issue #8: down-day cells with
// NBSP and/or interior percent whitespace must parse as negative change,
// never MarkupDriftError and never a silent positive (latent sign inversion).
func TestParseStockDataDownDayLiveMarkup(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		wantAbs float64
		wantPct float64
	}{
		{"nbsp_and_spaced_pct", stockDownLiveNBSPFixture, -0.040, -1.32},
		{"ascii_spaced_pct", stockDownASCIISpacedFixture, -0.040, -1.32},
		{"entity_nbsp_multiline", stockDownFixture, -0.54, -4.43},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, err := ParseStockData(tc.fixture)
			if err != nil {
				t.Fatalf("ParseStockData: %v", err)
			}
			if v := fp(t, snap.Change, "Change"); v != tc.wantAbs {
				t.Errorf("Change = %v, want %v", v, tc.wantAbs)
			}
			if v := fp(t, snap.PctChange, "PctChange"); v != tc.wantPct {
				t.Errorf("PctChange = %v, want %v", v, tc.wantPct)
			}
			if snap.Change != nil && *snap.Change > 0 {
				t.Errorf("down-day Change must not be positive (latent sign inversion): %v", *snap.Change)
			}
		})
	}
}

func TestParseStockDataClosedSessionBlankChange(t *testing.T) {
	snap, err := ParseStockData(stockClosedFixture)
	if err != nil {
		t.Fatalf("ParseStockData: %v", err)
	}
	if snap.Change != nil || snap.PctChange != nil {
		t.Errorf("blank change cell must stay nil (closed session), got %v/%v", snap.Change, snap.PctChange)
	}
}

func TestParseStockDataShellPage(t *testing.T) {
	_, err := ParseStockData(shellStockFixture)
	var shellErr *ShellPageError
	if !errors.As(err, &shellErr) {
		t.Fatalf("shell page must be *ShellPageError, got %v", err)
	}
}

func TestParseStockDataChallenge(t *testing.T) {
	_, err := ParseStockData(challengeFixture)
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("challenge page must be *ChallengeError, got %v", err)
	}
}

func TestParseComposite(t *testing.T) {
	comp, err := ParseComposite(compositeFixture)
	if err != nil {
		t.Fatalf("ParseComposite: %v", err)
	}
	if len(comp.Indices) != 3 {
		t.Fatalf("got %d indices, want 3", len(comp.Indices))
	}
	psei := comp.Indices[0]
	if psei.Code != "PSEI" || psei.Value != 6314.9 || psei.Change != 33.89 || psei.PctChange != 0.54 {
		t.Errorf("PSEI = %+v", psei)
	}
	if psei.TradeDate != "2026-07-27T14:50:00+08:00" {
		t.Errorf("PSEI TradeDate = %q (entity decode)", psei.TradeDate)
	}
	ind := comp.Indices[2]
	if ind.Code != "IND" || ind.Change != -3.65 {
		t.Errorf("IND = %+v (negative change parse)", ind)
	}
	if fp(t, comp.TotalVolume, "TotalVolume") != 448077665 {
		t.Errorf("TotalVolume = %v", *comp.TotalVolume)
	}
	if fp(t, comp.TotalValue, "TotalValue") != 4813139047.36 {
		t.Errorf("TotalValue = %v", *comp.TotalValue)
	}
	if comp.TotalTrades == nil || *comp.TotalTrades != 69416 {
		t.Errorf("TotalTrades = %v", comp.TotalTrades)
	}
	if comp.Advances == nil || *comp.Advances != 94 {
		t.Errorf("Advances = %v", comp.Advances)
	}
	if comp.Declines == nil || *comp.Declines != 75 {
		t.Errorf("Declines = %v", comp.Declines)
	}
	if comp.Unchanged == nil || *comp.Unchanged != 39 {
		t.Errorf("Unchanged = %v", comp.Unchanged)
	}
}

func TestParseCompositeShellPage(t *testing.T) {
	_, err := ParseComposite("<html><body>maintenance</body></html>")
	var shellErr *ShellPageError
	if !errors.As(err, &shellErr) {
		t.Fatalf("page without PSEI marker must be *ShellPageError, got %v", err)
	}
}

func TestParseCompositeChallenge(t *testing.T) {
	_, err := ParseComposite(challengeFixture)
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("challenge page must be *ChallengeError, got %v", err)
	}
}

func TestParsePSEISeries(t *testing.T) {
	rows, err := ParsePSEISeries(compositeFixture)
	if err != nil {
		t.Fatalf("ParsePSEISeries: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// 1627430400 = 2021-07-28T00:00:00Z
	if rows[0].Date != "2021-07-28" {
		t.Errorf("rows[0].Date = %q, want 2021-07-28 (epoch seconds -> UTC date)", rows[0].Date)
	}
	if rows[0].Close != 6473.03 {
		t.Errorf("rows[0].Close = %v", rows[0].Close)
	}
	// 1784851200 = 2026-07-24T00:00:00Z
	if rows[1].Date != "2026-07-24" {
		t.Errorf("rows[1].Date = %q, want 2026-07-24", rows[1].Date)
	}
}

func TestParsePSEISeriesMissingInput(t *testing.T) {
	_, err := ParsePSEISeries(`<html><body>no inputs</body></html>`)
	var shellErr *ShellPageError
	if !errors.As(err, &shellErr) {
		t.Fatalf("missing series input must be *ShellPageError, got %v", err)
	}
}

// TestParsePSEISeriesUTCDateMatchesManila pins the series date invariant:
// embedded epochs are exact midnight-UTC multiples, and midnight UTC + 8h
// stays on the SAME calendar date in Manila, so the UTC-formatted date can
// never shift versus Asia/Manila.
func TestParsePSEISeriesUTCDateMatchesManila(t *testing.T) {
	manila, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		manila = time.FixedZone("PHT", 8*60*60)
	}
	for _, epoch := range []int64{1627430400, 1784851200} {
		if epoch%86400 != 0 {
			t.Fatalf("epoch %d is not a midnight-UTC multiple; fixture assumption broken", epoch)
		}
		utcDate := time.Unix(epoch, 0).UTC().Format("2006-01-02")
		manilaDate := time.Unix(epoch, 0).In(manila).Format("2006-01-02")
		if utcDate != manilaDate {
			t.Errorf("epoch %d: UTC date %s != Manila date %s — invariant broken", epoch, utcDate, manilaDate)
		}
	}
	// And the known epoch maps to its expected date through the parser.
	rows, err := ParsePSEISeries(compositeFixture)
	if err != nil {
		t.Fatalf("ParsePSEISeries: %v", err)
	}
	if rows[0].Date != "2021-07-28" {
		t.Errorf("rows[0].Date = %q, want 2021-07-28", rows[0].Date)
	}
}

// stockDriftFixture has a NON-blank change cell no known pattern matches:
// upstream markup drift, which must be a typed error — never a silent nil
// that would masquerade as a closed session.
const stockDriftFixture = `<script>
sendData.cmpy_id = "34";
sendData.security_id = "320";
</script>
<div class="compInfo"><p>Atlas Consolidated Mining and Development Corporation</p></div>
<option value="320" selected>AT</option>
<table class="view">
<tr>
  <th>Change(% Change)</th>
  <td>
  higher by half a peso
  </td>
</tr>
</table>`

func TestParseStockDataChangeCellMarkupDrift(t *testing.T) {
	_, err := ParseStockData(stockDriftFixture)
	var driftErr *MarkupDriftError
	if !errors.As(err, &driftErr) {
		t.Fatalf("non-blank unmatched change cell must be *MarkupDriftError, got %v", err)
	}
	if driftErr.Field != "Change(% Change)" {
		t.Errorf("Field = %q", driftErr.Field)
	}
}

func TestCleanTextHardensIssuerStrings(t *testing.T) {
	// 10k-rune title with embedded control characters: comes out capped at
	// 500 runes + ellipsis with controls stripped and whitespace collapsed.
	long := strings.Repeat("A\x00\x1b[31mB\t\tC  ", 1000)
	got := cleanText(long)
	if runes := []rune(got); len(runes) != 501 {
		t.Fatalf("len = %d runes, want 501 (500 + ellipsis)", len(runes))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated output must end with ellipsis, got %q", got[len(got)-8:])
	}
	for _, r := range got {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control character %q survived cleanText", r)
		}
	}
	if strings.Contains(got, "  ") {
		t.Errorf("whitespace runs must collapse, got %q", got[:40])
	}
	// Short benign strings pass through (entities decoded, trimmed).
	if got := cleanText("  GT Capital &amp; Co.  "); got != "GT Capital & Co." {
		t.Errorf("cleanText benign = %q", got)
	}
	if got := CleanText("a\nb"); got != "a b" {
		t.Errorf("CleanText newline = %q", got)
	}
}

// profileFixture is a trimmed companyInformation/form.do fragment.
const profileFixture = `<div class="compInfo">
  <p style="">GT Capital Holdings, Inc.</p>
</div>
<table class="view">
<tr><th>Sector</th><td>Holding Firms</td><th>Subsector</th><td>Holding Firms</td></tr>
<tr><th>Incorporation Date</th><td>Jul 26, 2007</td><th>Fiscal Year</th><td>12/31 <span>(Month/Day)</span></td></tr>
<tr><th>External Auditor</th><td>SyCip Gorres Velayo &amp; Co.</td><th>Corporate Life</th><td>Perpetual</td></tr>
<tr><th>Number of Directors</th><td>11 Directors</td><th>Website</th><td>www.gtcapital.com.ph</td></tr>
</table>`

func TestParseCompanyProfileTable(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr any // nil, or pointer to expected typed error
		check   func(t *testing.T, p *CompanyProfile)
	}{
		{
			name: "happy path",
			body: profileFixture,
			check: func(t *testing.T, p *CompanyProfile) {
				if p.Name != "GT Capital Holdings, Inc." || p.Sector != "Holding Firms" {
					t.Errorf("name/sector = %q/%q", p.Name, p.Sector)
				}
				if p.IncorporationDate != "2007-07-26" {
					t.Errorf("IncorporationDate = %q", p.IncorporationDate)
				}
				if p.FiscalYear != "12/31" {
					t.Errorf("FiscalYear = %q", p.FiscalYear)
				}
				if p.Auditor != "SyCip Gorres Velayo & Co." {
					t.Errorf("Auditor = %q", p.Auditor)
				}
				if p.Directors == nil || *p.Directors != 11 {
					t.Errorf("Directors = %v", p.Directors)
				}
			},
		},
		{name: "shell page (no Sector row)", body: `<html><body><table><tr><th>Other</th><td>x</td></tr></table></body></html>`, wantErr: &ShellPageError{}},
		{name: "challenge page", body: challengeFixture, wantErr: &ChallengeError{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseCompanyProfile(tc.body)
			switch want := tc.wantErr.(type) {
			case nil:
				if err != nil {
					t.Fatalf("ParseCompanyProfile: %v", err)
				}
				tc.check(t, p)
			case *ShellPageError:
				var e *ShellPageError
				if !errors.As(err, &e) {
					t.Fatalf("want *ShellPageError, got %v", err)
				}
			case *ChallengeError:
				var e *ChallengeError
				if !errors.As(err, &e) {
					t.Fatalf("want *ChallengeError, got %v", err)
				}
			default:
				t.Fatalf("unhandled wantErr %T", want)
			}
		})
	}
}

func TestParseAutocompleteTable(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   bool
		challenge bool
		want      []AutocompleteMatch
	}{
		{
			name: "happy path",
			body: `[{"cmpyId":"633","cmpyNm":"GT Capital Holdings, Inc.","symbol":"GTCAP","etfYn":"0"},{"cmpyId":"19","cmpyNm":"Atok-Big Wedge Co., Inc.","symbol":"AB","etfYn":"0"}]`,
			want: []AutocompleteMatch{
				{CmpyID: 633, Name: "GT Capital Holdings, Inc.", Symbol: "GTCAP"},
				{CmpyID: 19, Name: "Atok-Big Wedge Co., Inc.", Symbol: "AB"},
			},
		},
		{name: "non-numeric cmpyId rows are skipped", body: `[{"cmpyId":"abc","cmpyNm":"Bogus","symbol":"X","etfYn":"0"}]`, want: []AutocompleteMatch{}},
		{name: "malformed body", body: `<html>not json</html>`, wantErr: true},
		{name: "challenge page", body: challengeFixture, wantErr: true, challenge: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAutocomplete([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if tc.challenge {
					var e *ChallengeError
					if !errors.As(err, &e) {
						t.Fatalf("want *ChallengeError, got %v", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAutocomplete: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d matches, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("match[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseMarketCapTable(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *float64
	}{
		{
			name: "happy path",
			body: `<table><tr><th>Market Capitalization</th><td> 41,917,824,132.60</td></tr></table>`,
			want: func() *float64 { v := 41917824132.60; return &v }(),
		},
		{name: "blank cell", body: `<table><tr><th>Market Capitalization</th><td> </td></tr></table>`, want: nil},
		{name: "row absent (shell input)", body: `<html><body>nothing here</body></html>`, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMarketCap(tc.body)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("want nil, got %v", *got)
			case tc.want != nil && (got == nil || *got != *tc.want):
				t.Errorf("got %v, want %v", got, *tc.want)
			}
		})
	}
}
