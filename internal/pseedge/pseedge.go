// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Package pseedge parses PSE Edge (edge.pse.com.ph) and frames.pse.com.ph
// HTML/JSON payloads into typed rows. Every parser hard-fails with a typed
// error on challenge pages (Cloudflare interstitials) and empty shell pages
// (HTTP 200 bodies missing the required markers), so callers can never
// mistake a blocked or bogus response for an empty-but-valid result.
//
// Endpoint contracts are documented in BUILD-CONTEXT.md §1–§4 and were
// live-verified 2026-07-27.
package pseedge

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ChallengeError is returned when a response body is a bot-protection
// challenge page rather than real content. Typed so callers can exit with
// a distinct error class instead of an empty success.
type ChallengeError struct {
	Endpoint string
	Marker   string
}

func (e *ChallengeError) Error() string {
	return fmt.Sprintf("pse-edge %s: bot-protection challenge page detected (marker %q); retry later or from a different network", e.Endpoint, e.Marker)
}

// ShellPageError is returned when a 200 response is an empty shell —
// headers or scaffolding present but the required data markers absent
// (e.g. a bogus cmpy_id, or a served-but-empty fragment).
type ShellPageError struct {
	Endpoint string
	Reason   string
}

func (e *ShellPageError) Error() string {
	return fmt.Sprintf("pse-edge %s: page is an empty shell (%s); refusing to emit empty data", e.Endpoint, e.Reason)
}

// MarkupDriftError is returned when a required cell is present with
// non-blank content that no known pattern matches — upstream markup has
// drifted. Typed so callers can distinguish it from a legitimately blank
// cell (which parses to nil) instead of silently serving nulls.
type MarkupDriftError struct {
	Endpoint string
	Field    string
	Content  string
}

func (e *MarkupDriftError) Error() string {
	return fmt.Sprintf("pse-edge %s: %s cell has unrecognized content %q (markup drift?); refusing to serve it as null", e.Endpoint, e.Field, e.Content)
}

// challengeMarkers are substrings that identify Cloudflare (and similar)
// challenge interstitials.
var challengeMarkers = []string{
	"Just a moment",
	"cf-chl",
	"cf_chl_opt",
	"challenge-platform",
}

// detectChallenge returns a *ChallengeError when body looks like a
// challenge page, nil otherwise.
func detectChallenge(endpoint, body string) error {
	for _, m := range challengeMarkers {
		if strings.Contains(body, m) {
			return &ChallengeError{Endpoint: endpoint, Marker: m}
		}
	}
	return nil
}

// userAgent identifies this CLI's hand-rolled fetchers (the generated
// client carries its own copy). Bump alongside releases.
const userAgent = "github.com/ngpestelos/pse-edge-pp-cli/0.1.0"

// cleanTextMaxRunes caps issuer-controlled strings (titles, company names,
// templates, profile fields) before they reach agent context. 500 runes is
// far beyond any legitimate PSE field; longer input is truncated with "…".
const cleanTextMaxRunes = 500

// cleanText trims and entity-decodes HTML-extracted text, then hardens it
// for downstream (agent) consumption: ASCII control characters are dropped
// (these are all single-line fields), whitespace runs collapse to one
// space, and the result is hard-capped at cleanTextMaxRunes runes. Mirrors
// cliutil.CleanText; duplicated locally so this package stays free of CLI
// wiring dependencies.
func cleanText(s string) string {
	s = html.UnescapeString(s)
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if r < 0x20 || r == 0x7f { // ASCII control chars (incl. \n, \t) → space
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if runes := []rune(out); len(runes) > cleanTextMaxRunes {
		out = string(runes[:cleanTextMaxRunes]) + "…"
	}
	return out
}

// CleanText exposes cleanText for callers outside this package that emit
// issuer-controlled strings from non-HTML sources (e.g. the phisix JSON
// mirror) and need the same control-char/whitespace/length hardening.
func CleanText(s string) string { return cleanText(s) }

// parseFloatLoose parses a comma-grouped decimal ("111,851,478.00").
// Returns nil for empty/blank cells so nullable columns stay NULL.
func parseFloatLoose(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(cleanText(s), ",", ""))
	if s == "" || s == "-" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// ---------------------------------------------------------------------------
// §1 Company directory
// ---------------------------------------------------------------------------

// Company is one row of the companyDirectory listing.
type Company struct {
	CmpyID     int    `json:"cmpy_id"`
	SecurityID int    `json:"security_id"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
}

// directoryRowRE captures the name-anchor + symbol-anchor pair emitted per
// company row:
//
//	<td><a href="#company" onclick="cmDetail('55','347');return false;">Asia Amalgamated Holdings Corporation</a></td>
//	<td class="alignC"><a href="#company" onclick="cmDetail('55','347');return false;">AAA</a></td>
var directoryRowRE = regexp.MustCompile(
	`(?s)<td><a[^>]*onclick="cmDetail\('(\d+)','(\d+)'\);return false;">([^<]+)</a></td>\s*` +
		`<td class="alignC"><a[^>]*onclick="cmDetail\('\d+','\d+'\);return false;">([^<]+)</a></td>`)

// ParseDirectoryPage extracts company rows from one page of
// /companyDirectory/search.ax. pageNo is the 1-based page that was
// requested: zero rows on page 1 is a hard error (challenge/shell page),
// while zero rows on a later page is the natural end-of-listing signal.
func ParseDirectoryPage(htmlBody string, pageNo int) ([]Company, error) {
	if err := detectChallenge("companyDirectory/search.ax", htmlBody); err != nil {
		return nil, err
	}
	matches := directoryRowRE.FindAllStringSubmatch(htmlBody, -1)
	companies := make([]Company, 0, len(matches))
	for _, m := range matches {
		cmpyID, err1 := strconv.Atoi(m[1])
		securityID, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		companies = append(companies, Company{
			CmpyID:     cmpyID,
			SecurityID: securityID,
			Name:       cleanText(m[3]),
			Symbol:     cleanText(m[4]),
		})
	}
	if len(companies) == 0 && pageNo <= 1 {
		if !strings.Contains(htmlBody, `<table class="list"`) {
			return nil, &ShellPageError{Endpoint: "companyDirectory/search.ax", Reason: "page 1 has no company listing table"}
		}
		return nil, &ShellPageError{Endpoint: "companyDirectory/search.ax", Reason: "page 1 listing table has zero company rows"}
	}
	return companies, nil
}

// ---------------------------------------------------------------------------
// §2 DisclosureCht.ax history
// ---------------------------------------------------------------------------

// EODRow is one completed daily bar from DisclosureCht.ax. Value is peso
// value traded; the endpoint serves no share-volume field.
type EODRow struct {
	TradingDate string  `json:"trading_date"` // YYYY-MM-DD
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Close       float64 `json:"close"`
	Value       float64 `json:"value"`
}

// chartDateLayout is the CHART_DATE format the endpoint emits.
const chartDateLayout = "Jan 2, 2006 15:04:05"

type chartResponse struct {
	ChartData []struct {
		Open      float64 `json:"OPEN"`
		High      float64 `json:"HIGH"`
		Low       float64 `json:"LOW"`
		Close     float64 `json:"CLOSE"`
		Value     float64 `json:"VALUE"`
		ChartDate string  `json:"CHART_DATE"`
	} `json:"chartData"`
}

// ParseHistoryResponse decodes the DisclosureCht.ax JSON body into EOD
// rows. A non-JSON body is checked for challenge markers before failing.
func ParseHistoryResponse(body []byte) ([]EODRow, error) {
	var resp chartResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		if cErr := detectChallenge("common/DisclosureCht.ax", string(body)); cErr != nil {
			return nil, cErr
		}
		return nil, fmt.Errorf("pse-edge common/DisclosureCht.ax: response is not the expected chartData JSON: %w", err)
	}
	rows := make([]EODRow, 0, len(resp.ChartData))
	for _, d := range resp.ChartData {
		t, err := time.Parse(chartDateLayout, d.ChartDate)
		if err != nil {
			return nil, fmt.Errorf("pse-edge common/DisclosureCht.ax: unparseable CHART_DATE %q: %w", d.ChartDate, err)
		}
		rows = append(rows, EODRow{
			TradingDate: t.Format("2006-01-02"),
			Open:        d.Open,
			High:        d.High,
			Low:         d.Low,
			Close:       d.Close,
			Value:       d.Value,
		})
	}
	return rows, nil
}

// ValidateEOD rejects rows outside the sane-price band before they can
// reach the local store: non-positive prices, prices above 1e7, and
// zero/empty dates are all hard errors per the red-team contract.
func ValidateEOD(row EODRow) error {
	if row.TradingDate == "" {
		return fmt.Errorf("eod row rejected: empty trading date")
	}
	if t, err := time.Parse("2006-01-02", row.TradingDate); err != nil || t.IsZero() {
		return fmt.Errorf("eod row rejected: invalid trading date %q", row.TradingDate)
	}
	for _, p := range []struct {
		name string
		v    float64
	}{{"open", row.Open}, {"high", row.High}, {"low", row.Low}, {"close", row.Close}} {
		if p.v <= 0 {
			return fmt.Errorf("eod row %s rejected: %s price %v is not positive", row.TradingDate, p.name, p.v)
		}
		if p.v > 1e7 {
			return fmt.Errorf("eod row %s rejected: %s price %v exceeds sanity bound 1e7", row.TradingDate, p.name, p.v)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// §3 stockData.do snapshot
// ---------------------------------------------------------------------------

// Snapshot is the parsed companyPage/stockData.do page. All numeric fields
// are pointers so blank cells (non-trading day, thin issues) stay NULL in
// SQL rather than collapsing to zero.
type Snapshot struct {
	CmpyID     int    `json:"cmpy_id"`
	SecurityID int    `json:"security_id"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
	AsOf       string `json:"as_of"` // raw "Jul 27, 2026 02:50 PM" page stamp

	LastTradedPrice *float64 `json:"last_traded_price"`
	Open            *float64 `json:"open"`
	High            *float64 `json:"high"`
	Low             *float64 `json:"low"`
	PrevClose       *float64 `json:"prev_close"`
	PrevCloseDate   string   `json:"prev_close_date,omitempty"` // YYYY-MM-DD
	Change          *float64 `json:"change"`                    // signed; sign derived from up/down prefix
	PctChange       *float64 `json:"pct_change"`
	Volume          *float64 `json:"volume"`
	Value           *float64 `json:"value"`
	AvgPrice        *float64 `json:"avg_price"`
	Week52High      *float64 `json:"week52_high"`
	Week52Low       *float64 `json:"week52_low"`
}

var (
	sendCmpyIDRE     = regexp.MustCompile(`sendData\.cmpy_id\s*=\s*"(\d+)"`)
	sendSecurityIDRE = regexp.MustCompile(`(?:var\s+security_id\s*=\s*|sendData\.security_id\s*=\s*)"(\d+)"`)
	compInfoNameRE   = regexp.MustCompile(`(?s)<div class="compInfo">\s*<p[^>]*>([^<]+)</p>`)
	symbolOptionRE   = regexp.MustCompile(`<option value="\d+"\s+selected\s*>([^<]+)</option>`)
	asOfRE           = regexp.MustCompile(`As of\s+([A-Z][a-z]{2} \d{1,2}, \d{4}[^<]*)`)
	thTdRE           = regexp.MustCompile(`(?s)<th>([^<]+)</th>\s*<td[^>]*>(.*?)</td>`)
	// changeCellRE per BUILD-CONTEXT §3: absolute change is UNSIGNED; the
	// required up/down word prefix carries the sign. Prefix is mandatory so
	// an unmatched "down" cannot be skipped by an unanchored match that
	// then silently reports a positive change (issue #8). Percent group
	// allows interior whitespace ("( 1.32%)") as currently served by EDGE.
	// Callers must run normalizeChangeCell first (NBSP / &nbsp; → space).
	changeCellRE    = regexp.MustCompile(`(?is)(up|down)\s*([\d,\.]+)\s*\(\s*([\d,\.\-]+)\s*%\s*\)`)
	prevCloseDateRE = regexp.MustCompile(`\(([A-Z][a-z]{2} \d{1,2}, \d{4})\)`)
)

// normalizeChangeCell flattens EDGE change-cell markup for changeCellRE.
// PSE Edge has served U+00A0 between the direction word and the absolute
// change; Go's \s is ASCII-only and does not match it. Decoding &nbsp; /
// numeric entities and mapping NBSP to a regular space closes that gap
// without relying on the entity form remaining stable.
func normalizeChangeCell(raw string) string {
	s := html.UnescapeString(raw)
	// Explicit entity leftovers (if UnescapeString was skipped upstream)
	// and common numeric forms that may appear in scraped fragments.
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&#160;", " ")
	s = strings.ReplaceAll(s, "&#xA0;", " ")
	s = strings.ReplaceAll(s, "&#xa0;", " ")
	return strings.Map(func(r rune) rune {
		if r == '\u00a0' {
			return ' '
		}
		return r
	}, s)
}

// ParseStockData parses a companyPage/stockData.do page. Bogus company IDs
// return HTTP 200 with a ~1KB shell ("Stock symbol not found." message, no
// Change(% Change) header) — that is a typed hard error, never an empty
// Snapshot.
func ParseStockData(htmlBody string) (*Snapshot, error) {
	if err := detectChallenge("companyPage/stockData.do", htmlBody); err != nil {
		return nil, err
	}
	if strings.Contains(htmlBody, "Stock symbol not found") {
		return nil, &ShellPageError{Endpoint: "companyPage/stockData.do", Reason: `server message "Stock symbol not found."`}
	}
	if !strings.Contains(htmlBody, "Change(% Change)") {
		return nil, &ShellPageError{Endpoint: "companyPage/stockData.do", Reason: "required Change(% Change) header absent"}
	}

	snap := &Snapshot{}
	if m := sendCmpyIDRE.FindStringSubmatch(htmlBody); m != nil {
		snap.CmpyID, _ = strconv.Atoi(m[1])
	}
	if m := sendSecurityIDRE.FindStringSubmatch(htmlBody); m != nil {
		snap.SecurityID, _ = strconv.Atoi(m[1])
	}
	if m := compInfoNameRE.FindStringSubmatch(htmlBody); m != nil {
		snap.Name = cleanText(m[1])
	}
	if m := symbolOptionRE.FindStringSubmatch(htmlBody); m != nil {
		snap.Symbol = cleanText(m[1])
	}
	if m := asOfRE.FindStringSubmatch(htmlBody); m != nil {
		snap.AsOf = cleanText(m[1])
	}

	cells := map[string]string{}
	for _, m := range thTdRE.FindAllStringSubmatch(htmlBody, -1) {
		label := cleanText(m[1])
		if _, seen := cells[label]; !seen {
			cells[label] = m[2]
		}
	}

	snap.LastTradedPrice = parseFloatLoose(cells["Last Traded Price"])
	snap.Open = parseFloatLoose(cells["Open"])
	snap.High = parseFloatLoose(cells["High"])
	snap.Low = parseFloatLoose(cells["Low"])
	snap.Volume = parseFloatLoose(cells["Volume"])
	snap.Value = parseFloatLoose(cells["Value"])
	snap.AvgPrice = parseFloatLoose(cells["Average Price"])
	snap.Week52High = parseFloatLoose(cells["52-Week High"])
	snap.Week52Low = parseFloatLoose(cells["52-Week Low"])

	if raw, ok := cells["Previous Close and Date"]; ok {
		if m := prevCloseDateRE.FindStringSubmatch(raw); m != nil {
			if t, err := time.Parse("Jan 2, 2006", m[1]); err == nil {
				snap.PrevCloseDate = t.Format("2006-01-02")
			}
			raw = prevCloseDateRE.ReplaceAllString(raw, "")
		}
		snap.PrevClose = parseFloatLoose(raw)
	}

	// Change cell: sign derived from the required up/down prefix. A BLANK
	// cell is an explicit closed-session state — change fields stay nil,
	// never zero. A NON-blank cell the pattern cannot match is upstream
	// markup drift: a typed hard error, never a silent nil (callers could
	// not tell the two apart otherwise). Bare "up"/"down" with no figures
	// also stays nil (legitimate intermediate/blank state).
	if raw, ok := cells["Change(% Change)"]; ok {
		m := changeCellRE.FindStringSubmatch(normalizeChangeCell(raw))
		if m == nil {
			if flat := stripTags(raw); flat != "" && flat != "-" && !strings.EqualFold(flat, "up") && !strings.EqualFold(flat, "down") {
				return nil, &MarkupDriftError{Endpoint: "companyPage/stockData.do", Field: "Change(% Change)", Content: flat}
			}
		}
		if m != nil {
			abs := parseFloatLoose(m[2])
			pct := parseFloatLoose(strings.TrimSuffix(m[3], "%"))
			sign := 1.0
			if strings.EqualFold(m[1], "down") {
				sign = -1.0
			}
			if abs != nil {
				v := sign * *abs
				snap.Change = &v
			}
			if pct != nil {
				v := *pct
				// The percent figure may print unsigned too; apply the
				// same derived sign when it lacks an explicit minus.
				if sign < 0 && v > 0 {
					v = -v
				}
				snap.PctChange = &v
			}
		}
	}

	return snap, nil
}

// ---------------------------------------------------------------------------
// §4 compositeSector market page
// ---------------------------------------------------------------------------

// Index is one PSEi/sector index reading from a frames.pse.com.ph "-key"
// hidden input.
type Index struct {
	Code      string  `json:"index_code"`
	Group     string  `json:"index_group"`
	Value     float64 `json:"value"`
	Change    float64 `json:"change"`
	PctChange float64 `json:"pct_change"`
	TradeDate string  `json:"trade_date"` // ISO as served, e.g. 2026-07-27T14:50:00+08:00
}

// Composite is the parsed compositeSector page: index readings plus the
// market summary table. Summary fields are pointers — a missing table row
// stays NULL, never zero.
type Composite struct {
	Indices     []Index  `json:"indices"`
	TotalVolume *float64 `json:"total_volume"`
	TotalValue  *float64 `json:"total_value"`
	TotalTrades *int64   `json:"total_trades"`
	Advances    *int64   `json:"advances"`
	Declines    *int64   `json:"declines"`
	Unchanged   *int64   `json:"unchanged"`
}

// IndexRow is one daily PSEi OHLC bar from the PSEI-value-values embedded
// series (epoch-seconds keyed).
type IndexRow struct {
	Date   string  `json:"trading_date"` // YYYY-MM-DD (UTC date of the epoch stamp)
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

var (
	indexKeyInputRE = regexp.MustCompile(`<input id="([A-Za-z-]+)-key" name="val1" type="hidden" value="([^"]*)"`)
	summaryRowRE    = regexp.MustCompile(`(?s)<td class="[^"]*historical-body">\s*([^<]+?)\s*</td>\s*<td class="[^"]*historical-body">\s*([^<]*?)\s*</td>`)
	pseiSeriesRE    = regexp.MustCompile(`<input[^>]*id="PSEI-value-values"[^>]*value="([^"]*)"`)
)

type indexKeyJSON struct {
	ID            string `json:"Id"`
	SectorCode    string `json:"SectorCode"`
	IndexGroup    string `json:"IndexGroup"`
	Value         string `json:"Value"`
	Change        string `json:"Change"`
	PercentChange string `json:"PercentChange"`
	TradeDate     string `json:"tradeDate"`
}

// ParseComposite parses the frames.pse.com.ph/compositeSector page:
// PSEi + sector index readings from the single-encoded &quot; hidden
// inputs, and the market summary table (Total Volume/Trades/Value,
// Advances/Declines/Unchanged). A challenge page or a page without the
// PSEI marker is a typed error.
func ParseComposite(htmlBody string) (*Composite, error) {
	if err := detectChallenge("compositeSector", htmlBody); err != nil {
		return nil, err
	}
	if !strings.Contains(htmlBody, `id="PSEI-key"`) {
		return nil, &ShellPageError{Endpoint: "compositeSector", Reason: "PSEI marker absent"}
	}

	comp := &Composite{Indices: make([]Index, 0, 9)}
	for _, m := range indexKeyInputRE.FindAllStringSubmatch(htmlBody, -1) {
		decoded := html.UnescapeString(m[2])
		var k indexKeyJSON
		if err := json.Unmarshal([]byte(decoded), &k); err != nil {
			return nil, fmt.Errorf("pse-edge compositeSector: index input %q is not valid JSON after entity decode: %w", m[1], err)
		}
		value := parseFloatLoose(k.Value)
		change := parseFloatLoose(k.Change)
		pct := parseFloatLoose(k.PercentChange)
		idx := Index{Code: k.SectorCode, Group: k.IndexGroup, TradeDate: k.TradeDate}
		if value != nil {
			idx.Value = *value
		}
		if change != nil {
			idx.Change = *change
		}
		if pct != nil {
			idx.PctChange = *pct
		}
		comp.Indices = append(comp.Indices, idx)
	}

	for _, m := range summaryRowRE.FindAllStringSubmatch(htmlBody, -1) {
		label := cleanText(m[1])
		switch label {
		case "Total Volume":
			comp.TotalVolume = parseFloatLoose(m[2])
		case "Total Value (in PHP)", "Total Value":
			comp.TotalValue = parseFloatLoose(m[2])
		case "Total Trades", "No. of Trades":
			comp.TotalTrades = parseIntLoose(m[2])
		case "Advances":
			comp.Advances = parseIntLoose(m[2])
		case "Declines":
			comp.Declines = parseIntLoose(m[2])
		case "Unchanged":
			comp.Unchanged = parseIntLoose(m[2])
		}
	}

	return comp, nil
}

func parseIntLoose(s string) *int64 {
	f := parseFloatLoose(s)
	if f == nil {
		return nil
	}
	n := int64(*f)
	return &n
}

type seriesPoint struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// ParsePSEISeries extracts the daily PSEi OHLC series embedded in the
// PSEI-value-values hidden input (value is a &quot;-encoded [[{...}]]
// double-wrapped array; time is epoch seconds, rendered as the UTC date).
func ParsePSEISeries(htmlBody string) ([]IndexRow, error) {
	if err := detectChallenge("compositeSector", htmlBody); err != nil {
		return nil, err
	}
	m := pseiSeriesRE.FindStringSubmatch(htmlBody)
	if m == nil {
		return nil, &ShellPageError{Endpoint: "compositeSector", Reason: "PSEI-value-values series input absent"}
	}
	decoded := html.UnescapeString(m[1])
	var wrapped [][]seriesPoint
	if err := json.Unmarshal([]byte(decoded), &wrapped); err != nil {
		// Some renders may serve the series unwrapped; accept both shapes.
		var flat []seriesPoint
		if err2 := json.Unmarshal([]byte(decoded), &flat); err2 != nil {
			return nil, fmt.Errorf("pse-edge compositeSector: PSEI series is not valid JSON after entity decode: %w", err)
		}
		wrapped = [][]seriesPoint{flat}
	}
	if len(wrapped) == 0 {
		return []IndexRow{}, nil
	}
	points := wrapped[0]
	rows := make([]IndexRow, 0, len(points))
	for _, p := range points {
		if p.Time <= 0 {
			continue
		}
		// Invariant (verified by TestParsePSEISeriesUTCDateMatchesManila):
		// the embedded epochs are exact midnight-UTC multiples (t%86400==0),
		// and midnight UTC + 8h is the SAME calendar date in Manila, so UTC
		// date formatting cannot shift the trading date vs Asia/Manila.
		rows = append(rows, IndexRow{
			Date:   time.Unix(p.Time, 0).UTC().Format("2006-01-02"),
			Open:   p.Open,
			High:   p.High,
			Low:    p.Low,
			Close:  p.Close,
			Volume: p.Volume,
		})
	}
	return rows, nil
}
