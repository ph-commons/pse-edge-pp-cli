// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"os"
	"testing"
)

func TestParseDisclosureViewerLODE17Q(t *testing.T) {
	data, err := os.ReadFile("testdata/disclosure_viewer_lode_17q.html")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	edge := "2bc053ab3b1339fb64d70b69f0a3140b"
	v, err := ParseDisclosureViewer(edge, string(data))
	if err != nil {
		t.Fatalf("ParseDisclosureViewer: %v", err)
	}
	if v.EdgeNo != edge {
		t.Errorf("edge_no = %q", v.EdgeNo)
	}
	if v.Company != "Lodestar Investment Holdings Corporation" {
		t.Errorf("company = %q", v.Company)
	}
	if v.DisclosureDate != "2026-07-22" {
		t.Errorf("disclosure_date = %q, want 2026-07-22", v.DisclosureDate)
	}
	if v.Title != "Quarterly Report" {
		t.Errorf("title = %q, want Quarterly Report", v.Title)
	}
	if v.DocumentFileID != "1946761" {
		t.Errorf("document_file_id = %q", v.DocumentFileID)
	}
	if len(v.Attachments) != 1 || v.Attachments[0].FileID != "1946762" {
		t.Errorf("attachments = %+v", v.Attachments)
	}
	if want := ViewerURL(edge); v.ViewerURL != want {
		t.Errorf("viewer_url = %q, want %q", v.ViewerURL, want)
	}
	if v.Source != "edge_viewer" {
		t.Errorf("source = %q", v.Source)
	}
}

func TestParseDisclosureViewerShell(t *testing.T) {
	_, err := ParseDisclosureViewer("abc", "<html><body>no header</body></html>")
	if err == nil {
		t.Fatal("expected ShellPageError")
	}
	if _, ok := err.(*ShellPageError); !ok {
		t.Fatalf("got %T, want *ShellPageError", err)
	}
}
