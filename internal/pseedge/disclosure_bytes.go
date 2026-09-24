// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ph-commons/pse-edge-pp-cli/internal/cliutil"
)

// disclosureDocumentMaxBody is the largest original document this command will retain.
// A response that does not fit is an error. The prefix is not stored as the document.
var disclosureDocumentMaxBody int64 = 32 << 20

// DisclosurePayloadError is a downloaded response that is not a filing document.
// Kind is "unavailable" or "rejected_error_page".
type DisclosurePayloadError struct {
	Kind   string
	Reason string
}

func (e *DisclosurePayloadError) Error() string {
	return fmt.Sprintf("pse-edge downloadHtml.do: %s (%s)", e.Kind, e.Reason)
}

// DocumentURL is the official downloadHtml.do URL for one file id.
func DocumentURL(fileID string) string {
	return "https://edge.pse.com.ph/downloadHtml.do?file_id=" + url.QueryEscape(fileID)
}

// ValidDocumentFileID reports whether fileID is safe to place in a URL and a path.
func ValidDocumentFileID(fileID string) bool {
	if fileID == "" || len(fileID) > 20 {
		return false
	}
	for _, r := range fileID {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ClassifyDisclosurePayload accepts original document bytes.
// An empty body, a bot-challenge page, or an HTML error page is refused.
// PDF is detected from %PDF- magic, not from the Content-Type header alone.
func ClassifyDisclosurePayload(body []byte, headerContentType string) (string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", &DisclosurePayloadError{Kind: "unavailable", Reason: "empty body"}
	}
	if err := detectChallenge("downloadHtml.do", string(body)); err != nil {
		return "", &DisclosurePayloadError{Kind: "rejected_error_page", Reason: err.Error()}
	}
	trimmed := bytes.TrimLeft(body, "\r\n\t ")
	if bytes.HasPrefix(trimmed, []byte("%PDF-")) {
		return "application/pdf", nil
	}
	header := strings.ToLower(headerContentType)
	if strings.Contains(header, "application/pdf") {
		return "", &DisclosurePayloadError{Kind: "rejected_error_page", Reason: "content-type pdf without %PDF- magic"}
	}
	lower := strings.ToLower(string(body))
	for _, marker := range []string{"<title>error", "page not found", "an error occurred", "system error"} {
		if strings.Contains(lower, marker) {
			return "", &DisclosurePayloadError{Kind: "rejected_error_page", Reason: marker}
		}
	}
	return "text/html", nil
}

// FetchDisclosureDocument GETs downloadHtml.do and returns the raw body.
// Classification of error pages is the caller's job so bytes and outcomes stay distinct.
func FetchDisclosureDocument(ctx context.Context, hc *http.Client, fileID string) ([]byte, string, error) {
	if !ValidDocumentFileID(fileID) {
		return nil, "", fmt.Errorf("pse-edge downloadHtml.do: invalid file_id %q", fileID)
	}
	endpoint := edgeOrigin() + "/downloadHtml.do"
	u := endpoint + "?file_id=" + url.QueryEscape(fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("pse-edge downloadHtml.do: building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if hc == nil {
		hc = http.DefaultClient
	}
	disclosureLimiter.Wait()
	resp, err := hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("pse-edge downloadHtml.do file_id %s: %w", fileID, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, disclosureDocumentMaxBody+1))
	if err != nil {
		return nil, "", fmt.Errorf("pse-edge downloadHtml.do file_id %s: reading response: %w", fileID, err)
	}
	if int64(len(body)) > disclosureDocumentMaxBody {
		return nil, resp.Header.Get("Content-Type"), fmt.Errorf("pse-edge downloadHtml.do file_id %s: response exceeds %d bytes; refusing a truncated document", fileID, disclosureDocumentMaxBody)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		disclosureLimiter.OnRateLimit()
		retryAfter, _ := time.ParseDuration(strings.TrimSpace(resp.Header.Get("Retry-After")) + "s")
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, "", &cliutil.RateLimitError{URL: endpoint, RetryAfter: retryAfter, Body: preview}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.Header.Get("Content-Type"), &DisclosurePayloadError{Kind: "unavailable", Reason: "HTTP 404"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header.Get("Content-Type"), fmt.Errorf("pse-edge downloadHtml.do file_id %s: HTTP %d", fileID, resp.StatusCode)
	}
	disclosureLimiter.OnSuccess()
	return body, resp.Header.Get("Content-Type"), nil
}
