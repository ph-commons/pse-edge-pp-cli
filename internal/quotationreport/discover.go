// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultListingURL = "https://www.pse.com.ph/market-report/"
	eodCategorySlug   = "end-of-day-quotes"
	maxPDFBytes       = 20 << 20
	maxPageBytes      = 5 << 20
	ajaxPageSize      = 100
	ajaxPageCap       = 3
	userAgent         = "github.com/ph-commons/pse-edge-pp-cli/quotation-report"
)

var (
	paramsRe  = regexp.MustCompile(`var posts_table_params = (\{.*?\});`)
	tableIDRe = regexp.MustCompile(`id="(ptp_[A-Za-z0-9_]+)" class="posts-data-table"`)
	slugRe    = regexp.MustCompile(`data-slug="([^"]+)"`)
	hrefRe    = regexp.MustCompile(`href="([^"]+)"`)
)

// PDF is one downloaded report body. The URL came from a listing row.
type PDF struct {
	URL  string
	Body []byte
	SHA  string
}

// Found is a listing match and its PDF bytes.
type Found struct {
	Discovery Discovery
	PDFs      []PDF
}

// FindReport reads the official market-report listing and downloads the
// End of Day Quotes PDF whose title date is session. It does not invent a
// PDF URL when the listing does not name one.
func FindReport(ctx context.Context, listingURL string, session time.Time) (Found, error) {
	if listingURL == "" {
		listingURL = DefaultListingURL
	}
	listing, err := url.Parse(listingURL)
	if err != nil || listing.Host == "" {
		return Found{}, statusErr(StatusListingUnavailable, "listing URL was invalid")
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return statusErr(StatusAcquisitionFailed, "too many redirects")
			}
			return allowReportURL(req.URL, listing)
		},
	}
	page, err := getBytes(ctx, client, http.MethodGet, listingURL, nil, "", maxPageBytes, StatusListingUnavailable)
	if err != nil {
		return Found{}, err
	}
	ajaxURL, nonce, action, tableID, err := listingParams(page)
	if err != nil {
		return Found{}, err
	}
	wantTitle := session.Format("January 2, 2006")
	var matches []ajaxRow
	for pageNo := 0; pageNo < ajaxPageCap; pageNo++ {
		rows, err := loadPosts(ctx, client, listing, ajaxURL, nonce, action, tableID, pageNo*ajaxPageSize)
		if err != nil {
			return Found{}, err
		}
		if len(rows) == 0 {
			break
		}
		older := 0
		dated := 0
		for _, row := range rows {
			title := strings.TrimSpace(row.Title)
			if title == wantTitle && categorySlug(row.Categories) == eodCategorySlug {
				matches = append(matches, row)
			}
			if when, ok := parseTitleDate(title); ok {
				dated++
				if when.Before(session) {
					older++
				}
			}
		}
		if len(matches) > 0 || (dated > 0 && older == dated) {
			break
		}
	}
	if len(matches) == 0 {
		return Found{}, statusErr(StatusReportUnavailable, "no End of Day Quotes row titled "+wantTitle)
	}
	found := Found{Discovery: Discovery{
		ListingURL:   listingURL,
		AjaxURL:      ajaxURL,
		TableID:      tableID,
		MatchedTitle: wantTitle,
		MatchedSlug:  eodCategorySlug,
		SessionTitle: wantTitle,
	}}
	seenURL := map[string]bool{}
	for _, row := range matches {
		if found.Discovery.UploadDate == "" {
			found.Discovery.UploadDate = strings.TrimSpace(row.Date)
		}
		rawHref := firstHref(row.Content)
		if rawHref == "" {
			continue
		}
		pdfURL, err := resolvePDFURL(listing, ajaxURL, rawHref)
		if err != nil {
			return Found{}, err
		}
		if seenURL[pdfURL] {
			continue
		}
		seenURL[pdfURL] = true
		body, err := getBytes(ctx, client, http.MethodGet, pdfURL, nil, "", maxPDFBytes, StatusAcquisitionFailed)
		if err != nil {
			return Found{}, err
		}
		if len(body) < 5 || string(body[:5]) != "%PDF-" {
			return Found{}, statusErr(StatusMalformedDocument, "download was not a PDF")
		}
		sum := sha256.Sum256(body)
		found.PDFs = append(found.PDFs, PDF{URL: pdfURL, Body: body, SHA: hex.EncodeToString(sum[:])})
	}
	if len(found.PDFs) == 0 {
		return Found{}, statusErr(StatusReportUnavailable, "End of Day Quotes row had no PDF link")
	}
	found.Discovery.PDFURL = found.PDFs[0].URL
	if len(uniqueSHA(found.PDFs)) > 1 {
		return found, statusErr(StatusConflictingListing, "listing returned more than one PDF hash for "+wantTitle)
	}
	return found, nil
}

func uniqueSHA(pdfs []PDF) []string {
	var out []string
	seen := map[string]bool{}
	for _, pdf := range pdfs {
		if seen[pdf.SHA] {
			continue
		}
		seen[pdf.SHA] = true
		out = append(out, pdf.SHA)
	}
	return out
}

func listingParams(page []byte) (ajaxURL, nonce, action, tableID string, err error) {
	params := paramsRe.FindSubmatch(page)
	table := tableIDRe.FindSubmatch(page)
	if params == nil || table == nil {
		return "", "", "", "", statusErr(StatusListingUnavailable, "listing page did not include table parameters")
	}
	var payload struct {
		AjaxURL    string `json:"ajax_url"`
		AjaxNonce  string `json:"ajax_nonce"`
		AjaxAction string `json:"ajax_action"`
	}
	if err := json.Unmarshal(params[1], &payload); err != nil {
		return "", "", "", "", statusErr(StatusListingUnavailable, "listing parameters were not JSON")
	}
	if payload.AjaxURL == "" || payload.AjaxNonce == "" || payload.AjaxAction == "" {
		return "", "", "", "", statusErr(StatusListingUnavailable, "listing parameters were incomplete")
	}
	if _, err := url.Parse(payload.AjaxURL); err != nil {
		return "", "", "", "", statusErr(StatusListingUnavailable, "listing ajax url was invalid")
	}
	return payload.AjaxURL, payload.AjaxNonce, payload.AjaxAction, string(table[1]), nil
}

type ajaxRow struct {
	Title      string `json:"title"`
	Categories string `json:"categories"`
	Date       string `json:"date"`
	Content    string `json:"content"`
}

func loadPosts(ctx context.Context, client *http.Client, listing *url.URL, ajaxURL, nonce, action, tableID string, start int) ([]ajaxRow, error) {
	form := url.Values{}
	form.Set("action", action)
	form.Set("_ajax_nonce", nonce)
	form.Set("table_id", tableID)
	form.Set("draw", "1")
	form.Set("start", fmt.Sprintf("%d", start))
	form.Set("length", fmt.Sprintf("%d", ajaxPageSize))
	form.Set("search[value]", "")
	form.Set("search[regex]", "false")
	form.Set("order[0][column]", "2")
	form.Set("order[0][dir]", "desc")
	names := []string{"title", "categories", "date", "content"}
	for i, name := range names {
		form.Set(fmt.Sprintf("columns[%d][data]", i), name)
		form.Set(fmt.Sprintf("columns[%d][name]", i), name)
		form.Set(fmt.Sprintf("columns[%d][searchable]", i), "true")
		form.Set(fmt.Sprintf("columns[%d][orderable]", i), "true")
		search := ""
		if name == "categories" {
			search = eodCategorySlug
		}
		form.Set(fmt.Sprintf("columns[%d][search][value]", i), search)
		form.Set(fmt.Sprintf("columns[%d][search][regex]", i), "false")
	}
	body, err := getBytes(ctx, client, http.MethodPost, ajaxURL, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", maxPageBytes, StatusListingUnavailable)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []ajaxRow `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, statusErr(StatusListingUnavailable, "listing response was not JSON")
	}
	return payload.Data, nil
}

func categorySlug(categories string) string {
	match := slugRe.FindStringSubmatch(categories)
	if match == nil {
		return ""
	}
	return match[1]
}

func firstHref(content string) string {
	match := hrefRe.FindStringSubmatch(content)
	if match == nil {
		return ""
	}
	return html.UnescapeString(match[1])
}

func parseTitleDate(title string) (time.Time, bool) {
	when, err := time.Parse("January 2, 2006", strings.TrimSpace(title))
	if err != nil {
		return time.Time{}, false
	}
	return when, true
}

func resolvePDFURL(listing *url.URL, base, href string) (string, error) {
	ref, err := url.Parse(href)
	if err != nil {
		return "", statusErr(StatusAcquisitionFailed, "PDF link was not a URL")
	}
	parent, err := url.Parse(base)
	if err != nil {
		return "", statusErr(StatusListingUnavailable, "listing ajax url was invalid")
	}
	resolved := parent.ResolveReference(ref)
	if err := allowReportURL(resolved, listing); err != nil {
		return "", err
	}
	return resolved.String(), nil
}

func allowReportURL(u, listing *url.URL) error {
	if u == nil || listing == nil || u.Host == "" {
		return statusErr(StatusAcquisitionFailed, "report URL was empty")
	}
	if u.Host == listing.Host && u.Scheme == listing.Scheme {
		return nil
	}
	if u.Scheme != "https" {
		return statusErr(StatusAcquisitionFailed, "report URL was not https")
	}
	switch u.Hostname() {
	case "www.pse.com.ph", "documents.pse.com.ph":
		return nil
	default:
		return statusErr(StatusAcquisitionFailed, "report URL host was not an official PSE host")
	}
}

func getBytes(ctx context.Context, client *http.Client, method, rawURL string, body io.Reader, contentType string, limit int, failStatus string) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if body != nil {
			if seeker, ok := body.(io.Seeker); ok && attempt > 0 {
				if _, err := seeker.Seek(0, io.SeekStart); err != nil {
					return nil, statusErr(failStatus, "could not retry request body")
				}
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
		if err != nil {
			return nil, statusErr(failStatus, "could not build request")
		}
		req.Header.Set("User-Agent", userAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
		resp.Body.Close()
		if readErr != nil {
			last = readErr
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, statusErr(failStatus, fmt.Sprintf("HTTP %d", resp.StatusCode))
		}
		if len(payload) > limit {
			return nil, statusErr(failStatus, "response exceeded the size bound")
		}
		return payload, nil
	}
	return nil, statusErr(failStatus, "request failed: "+last.Error())
}
