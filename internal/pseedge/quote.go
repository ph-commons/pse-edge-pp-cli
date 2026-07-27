// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Quote-support parsers: the autocomplete symbol-resolution endpoint and
// the Market Capitalization cell of stockData.do. Live-verified 2026-07-27:
//
//	GET /autoComplete/searchCompanyNameSymbol.ax?term=GTCAP
//	→ [{"cmpyId":"633","cmpyNm":"GT Capital Holdings, Inc.","symbol":"GTCAP","etfYn":"0"}]
//
// The endpoint returns the first ~20 alphabetical prefix matches only, so
// callers must exact-match the symbol field — never take the first row.

package pseedge

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// AutocompleteMatch is one row of /autoComplete/searchCompanyNameSymbol.ax.
type AutocompleteMatch struct {
	CmpyID int    `json:"cmpy_id"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	ETF    bool   `json:"etf"`
}

type autocompleteRow struct {
	CmpyID string `json:"cmpyId"`
	Name   string `json:"cmpyNm"`
	Symbol string `json:"symbol"`
	ETFYn  string `json:"etfYn"`
}

// ParseAutocomplete decodes the autocomplete JSON body. A non-JSON body is
// checked for challenge markers before failing.
func ParseAutocomplete(body []byte) ([]AutocompleteMatch, error) {
	var rows []autocompleteRow
	if err := json.Unmarshal(body, &rows); err != nil {
		if cErr := detectChallenge("autoComplete/searchCompanyNameSymbol.ax", string(body)); cErr != nil {
			return nil, cErr
		}
		return nil, fmt.Errorf("pse-edge autoComplete/searchCompanyNameSymbol.ax: response is not the expected JSON array: %w", err)
	}
	out := make([]AutocompleteMatch, 0, len(rows))
	for _, r := range rows {
		id, err := strconv.Atoi(r.CmpyID)
		if err != nil {
			continue
		}
		out = append(out, AutocompleteMatch{
			CmpyID: id,
			Name:   cleanText(r.Name),
			Symbol: cleanText(r.Symbol),
			ETF:    r.ETFYn == "1",
		})
	}
	return out, nil
}

// ParseMarketCap extracts the Market Capitalization cell from a
// companyPage/stockData.do page. Returns nil when the cell is blank or the
// row is absent. Callers should run ParseStockData first — that is where
// challenge/shell detection lives; this helper only reads one extra cell.
func ParseMarketCap(htmlBody string) *float64 {
	for _, m := range thTdRE.FindAllStringSubmatch(htmlBody, -1) {
		if cleanText(m[1]) == "Market Capitalization" {
			return parseFloatLoose(m[2])
		}
	}
	return nil
}
