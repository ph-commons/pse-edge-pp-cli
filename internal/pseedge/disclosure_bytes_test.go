// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyDisclosurePayload(t *testing.T) {
	pdf, err := ClassifyDisclosurePayload([]byte("%PDF-1.4\nhello"), "text/html")
	if err != nil || pdf != "application/pdf" {
		t.Fatalf("pdf = %s %v", pdf, err)
	}
	html, err := ClassifyDisclosurePayload([]byte("<html><body>Quarterly</body></html>"), "")
	if err != nil || html != "text/html" {
		t.Fatalf("html = %s %v", html, err)
	}
	_, err = ClassifyDisclosurePayload([]byte("  "), "")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("empty = %v", err)
	}
	_, err = ClassifyDisclosurePayload([]byte("<html><title>Error</title></html>"), "text/html")
	if err == nil || !strings.Contains(err.Error(), "rejected_error_page") {
		t.Fatalf("error page = %v", err)
	}
	_, err = ClassifyDisclosurePayload([]byte("<html>Just a moment</html>"), "")
	if err == nil || !strings.Contains(err.Error(), "rejected_error_page") {
		t.Fatalf("challenge = %v", err)
	}
}

func TestFetchDisclosureDocumentNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/downloadHtml.do" || r.URL.Query().Get("file_id") != "1959980" {
			t.Errorf("request = %s", r.URL)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("PSE_EDGE_BASE_URL", srv.URL)
	_, _, err := FetchDisclosureDocument(context.Background(), srv.Client(), "1959980")
	var payload *DisclosurePayloadError
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err = %v", err)
	}
	_ = payload
}
