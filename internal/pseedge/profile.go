// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

// Company-profile parser for /companyInformation/form.do?cmpy_id=N[&security_id=M].
// Live-verified 2026-07-27 against GTCAP (cmpy_id=633): profile fields are
// served as <th>Label</th><td>Value</td> rows inside table.view blocks, the
// company name in the compInfo header div.

package pseedge

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CompanyProfile is the parsed companyInformation/form.do page.
type CompanyProfile struct {
	Name              string `json:"name"`
	Sector            string `json:"sector"`
	Subsector         string `json:"subsector"`
	IncorporationDate string `json:"incorporation_date,omitempty"` // YYYY-MM-DD when parseable, else as printed
	FiscalYear        string `json:"fiscal_year,omitempty"`        // Month/Day, e.g. "12/31"
	Auditor           string `json:"auditor,omitempty"`
	CorporateLife     string `json:"corporate_life,omitempty"`
	Directors         *int   `json:"directors"`
	Website           string `json:"website,omitempty"`
}

// profileThTdRE is deliberately its own pattern (not thTdRE): profile pages
// emit <th> cells with attributes and nested markup in values.
var (
	profileThTdRE = regexp.MustCompile(`(?s)<th[^>]*>(.*?)</th>\s*<td[^>]*>(.*?)</td>`)
	tagRE         = regexp.MustCompile(`<[^>]+>`)
	spaceRunRE    = regexp.MustCompile(`\s+`)
)

// stripTags flattens an HTML fragment into normalized text.
func stripTags(s string) string {
	return cleanText(spaceRunRE.ReplaceAllString(tagRE.ReplaceAllString(s, " "), " "))
}

// ParseCompanyProfile parses a companyInformation/form.do page. A page
// without the Sector row is an empty shell (bogus cmpy_id) — typed hard
// error, never an empty profile.
func ParseCompanyProfile(htmlBody string) (*CompanyProfile, error) {
	if err := detectChallenge("companyInformation/form.do", htmlBody); err != nil {
		return nil, err
	}

	p := &CompanyProfile{}
	if m := compInfoNameRE.FindStringSubmatch(htmlBody); m != nil {
		p.Name = cleanText(m[1])
	}

	cells := map[string]string{}
	for _, m := range profileThTdRE.FindAllStringSubmatch(htmlBody, -1) {
		label := stripTags(m[1])
		if _, seen := cells[label]; !seen {
			cells[label] = stripTags(m[2])
		}
	}

	if _, ok := cells["Sector"]; !ok {
		return nil, &ShellPageError{Endpoint: "companyInformation/form.do", Reason: "required Sector row absent (bogus cmpy_id?)"}
	}

	p.Sector = cells["Sector"]
	p.Subsector = cells["Subsector"]
	p.Auditor = cells["External Auditor"]
	p.CorporateLife = cells["Corporate Life"]
	p.Website = cells["Website"]

	if raw := cells["Incorporation Date"]; raw != "" {
		if t, err := time.Parse("Jan 2, 2006", raw); err == nil {
			p.IncorporationDate = t.Format("2006-01-02")
		} else {
			p.IncorporationDate = raw
		}
	}
	// Fiscal Year prints as "12/31 (Month/Day)" once flattened — keep the
	// leading Month/Day token.
	if raw := cells["Fiscal Year"]; raw != "" {
		p.FiscalYear = strings.Fields(raw)[0]
	}
	if raw := cells["Number of Directors"]; raw != "" {
		if n, err := strconv.Atoi(strings.Fields(raw)[0]); err == nil {
			p.Directors = &n
		}
	}
	return p, nil
}
