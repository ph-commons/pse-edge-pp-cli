// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	symbolRe  = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,11}$`)
	measureRe = regexp.MustCompile(`^(?:-|\(?\d{1,3}(?:,\d{3})*(?:\.\d+)?\)?)$`)
	dateRe    = regexp.MustCompile(`^(January|February|March|April|May|June|July|August|September|October|November|December) ([1-9]|[12][0-9]|3[01]), ([12][0-9]{3})$`)
)

// ParseLayout reads pdftotext -layout output. expected is the session the
// caller asked for. A header date that disagrees is rejected and no rows
// are returned. Dashes stay null.
func ParseLayout(text string, expected time.Time, sha string) (Parsed, error) {
	wantDate := expected.Format("January 2, 2006")
	session := expected.Format("2006-01-02")
	pages := strings.Split(text, "\f")
	st := scanState{extract: true, board: "MAIN", class: "common", currency: "PHP"}
	var candidates []Row
	seenTitle := false
	seenDate := false

	for pageIdx, page := range pages {
		pageNo := pageIdx + 1
		lines := strings.Split(page, "\n")
		for lineNo, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if dateRe.MatchString(trimmed) {
				seenDate = true
				if trimmed != wantDate {
					return Parsed{}, statusErr(StatusWrongSessionDate, "report date "+trimmed+" is not "+wantDate)
				}
				continue
			}
			if strings.Contains(trimmed, "Daily Quotation Report") {
				seenTitle = true
				continue
			}
			noteSection(trimmed, &st)
			if !st.extract || strings.Contains(trimmed, "TOTAL") {
				continue
			}
			row, ok := parseQuoteLine(trimmed, pageNo, lineNo+1, session, sha, st)
			if ok {
				candidates = append(candidates, row)
			}
		}
	}
	if !seenTitle {
		return Parsed{}, statusErr(StatusMalformedDocument, "missing Daily Quotation Report heading")
	}
	if !seenDate {
		return Parsed{}, statusErr(StatusWrongSessionDate, "missing session date line")
	}
	if len(candidates) == 0 {
		return Parsed{}, statusErr(StatusPartialParse, "heading matched but no security rows were parsed")
	}
	parsed := collapseAmbiguous(candidates)
	parsed.SessionDate = session
	return parsed, nil
}

type scanState struct {
	extract  bool
	board    string
	class    string
	currency string
}

func noteSection(line string, st *scanState) {
	switch {
	case strings.Contains(line, "SECTORAL SUMMARY"),
		strings.Contains(line, "BLOCK SALES"),
		strings.Contains(line, "NO. OF ADVANCES"),
		strings.Contains(line, "Securities Under Suspension"),
		strings.Contains(line, "FOREIGN BUYING"):
		st.extract = false
	case line == "PREFERRED":
		st.extract, st.board, st.class, st.currency = true, "PREFERRED", "preferred", "PHP"
	case line == "WARRANTS":
		st.extract, st.board, st.class, st.currency = true, "WARRANTS", "warrant", "PHP"
	case strings.HasPrefix(line, "SMALL, MEDIUM") && !strings.Contains(line, "TOTAL"):
		st.extract, st.board, st.class, st.currency = true, "SME", "common", "PHP"
	case strings.HasPrefix(line, "EXCHANGE TRADED FUNDS") && !strings.Contains(line, "TOTAL"):
		st.extract, st.board, st.class, st.currency = true, "ETF", "etf", "PHP"
	case strings.Contains(line, "DOLLAR DENOMINATED"):
		st.extract, st.board, st.class, st.currency = true, "DDS", "dds", "USD"
	case strings.Contains(line, "MAIN BOARD"):
		st.extract, st.board, st.class, st.currency = true, "MAIN", "common", "PHP"
	case strings.Contains(line, "Value, USD"):
		st.currency = "USD"
	case strings.Contains(line, "Value, PHP"):
		st.currency = "PHP"
	}
}

func parseQuoteLine(line string, page, lineNo int, session, sha string, st scanState) (Row, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return Row{}, false
	}
	start := len(fields) - 9
	measures := make([]Measure, 9)
	for i := 0; i < 9; i++ {
		m, ok := parseMeasure(fields[start+i])
		if !ok {
			return Row{}, false
		}
		measures[i] = m
	}
	sym := fields[start-1]
	if !symbolRe.MatchString(sym) || start-1 == 0 {
		return Row{}, false
	}
	name := strings.Join(fields[:start-1], " ")
	if name == "" || strings.Contains(name, "TOTAL") {
		return Row{}, false
	}
	fieldsNames := []string{"bid", "ask", "open", "high", "low", "close", "volume", "value", "net_foreign"}
	status := make(map[string]string, len(fieldsNames))
	vals := make([]*float64, len(fieldsNames))
	for i, field := range fieldsNames {
		status[field] = measures[i].Status
		vals[i] = measures[i].Value
	}
	return Row{
		Symbol:        sym,
		IssueName:     name,
		Board:         st.board,
		SecurityClass: st.class,
		Currency:      st.currency,
		SessionDate:   session,
		SourceType:    SourceType,
		SourceSHA256:  sha,
		Page:          page,
		RowLocator:    "p" + strconv.Itoa(page) + ":l" + strconv.Itoa(lineNo) + ":" + sym,
		Bid:           vals[0],
		Ask:           vals[1],
		Open:          vals[2],
		High:          vals[3],
		Low:           vals[4],
		Close:         vals[5],
		Volume:        vals[6],
		Value:         vals[7],
		NetForeign:    vals[8],
		FieldStatus:   status,
	}, true
}

func parseMeasure(tok string) (Measure, bool) {
	if !measureRe.MatchString(tok) {
		return Measure{}, false
	}
	if tok == "-" {
		return Measure{Status: FieldReportedDash}, true
	}
	neg := strings.HasPrefix(tok, "(")
	raw := strings.Trim(tok, "()")
	raw = strings.ReplaceAll(raw, ",", "")
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return Measure{}, false
	}
	if neg {
		v = -v
	}
	return Measure{Value: &v, Status: FieldOK}, true
}

func collapseAmbiguous(rows []Row) Parsed {
	locators := map[string][]string{}
	order := make([]string, 0)
	for _, row := range rows {
		if _, ok := locators[row.Symbol]; !ok {
			order = append(order, row.Symbol)
		}
		locators[row.Symbol] = append(locators[row.Symbol], row.RowLocator)
	}
	var ambiguous []AmbiguousSymbol
	amb := map[string]bool{}
	for _, sym := range order {
		if len(locators[sym]) > 1 {
			amb[sym] = true
			ambiguous = append(ambiguous, AmbiguousSymbol{Symbol: sym, Locators: locators[sym]})
		}
	}
	kept := make([]Row, 0, len(rows))
	for _, row := range rows {
		if !amb[row.Symbol] {
			kept = append(kept, row)
		}
	}
	if ambiguous == nil {
		ambiguous = []AmbiguousSymbol{}
	}
	return Parsed{Rows: kept, Ambiguous: ambiguous}
}

// Cover matches a symbol request to parsed rows. Empty request means the
// whole admitted table. Absent is not the same as a present null close.
func Cover(parsed Parsed, requested []string) (Coverage, []Row, string) {
	by := map[string]Row{}
	for _, row := range parsed.Rows {
		by[row.Symbol] = row
	}
	ambSet := map[string]bool{}
	var ambNames []string
	for _, item := range parsed.Ambiguous {
		ambSet[item.Symbol] = true
		ambNames = append(ambNames, item.Symbol)
	}
	if ambNames == nil {
		ambNames = []string{}
	}
	cov := Coverage{
		Requested:        requested,
		Present:          []string{},
		Absent:           []string{},
		PresentNullClose: []string{},
		Ambiguous:        ambNames,
		RowCount:         len(parsed.Rows),
	}
	status := StatusOK
	if len(parsed.Ambiguous) > 0 {
		status = StatusAmbiguousIdentity
	}
	rows := parsed.Rows
	if len(requested) > 0 {
		rows = make([]Row, 0, len(requested))
		for _, sym := range requested {
			if ambSet[sym] {
				status = StatusAmbiguousIdentity
				continue
			}
			row, ok := by[sym]
			if !ok {
				cov.Absent = append(cov.Absent, sym)
				continue
			}
			cov.Present = append(cov.Present, sym)
			rows = append(rows, row)
			if row.Close == nil {
				cov.PresentNullClose = append(cov.PresentNullClose, sym)
			}
		}
		if status == StatusOK && len(cov.Absent) > 0 {
			status = StatusMissingSymbols
		}
	} else {
		for _, row := range parsed.Rows {
			if row.Close == nil {
				cov.PresentNullClose = append(cov.PresentNullClose, row.Symbol)
			}
		}
	}
	return cov, rows, status
}
