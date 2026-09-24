// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"os"
	"testing"
	"time"
)

func TestAdmitReplayAndRevision(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	pdf := PDF{URL: "https://documents.pse.com.ph/a.pdf", Body: []byte("%PDF-1\n"), SHA: "aaa"}
	parsed := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", Close: floatPtr(19.98)}}, Ambiguous: []AmbiguousSymbol{}}
	disc := Discovery{UploadDate: "September 24, 2026", SessionTitle: "September 24, 2026", PDFURL: pdf.URL}
	if _, _, _, err := Admit(root, session, pdf, disc, parsed, when); err != nil {
		t.Fatal(err)
	}
	if _, revision, _, err := Admit(root, session, pdf, disc, parsed, when.Add(time.Hour)); err != nil {
		t.Fatal(err)
	} else if revision {
		t.Fatal("same bytes created a revision")
	}
	idx, err := loadIndex(sessionDir(root, session))
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Revisions) != 1 || idx.CurrentSHA != "aaa" {
		t.Fatalf("index=%+v", idx)
	}
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	parsed.Rows[0].SourceSHA256 = "bbb"
	doc, revision, previous, err := Admit(root, session, next, disc, parsed, when.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !revision || previous != "aaa" || doc.Source.SHA256 != "bbb" {
		t.Fatalf("revision=%v previous=%s doc=%+v", revision, previous, doc.Source)
	}
	if _, err := os.Stat(sessionDir(root, session) + "/aaa.pdf"); err != nil {
		t.Fatal("previous pdf was dropped")
	}
	current, ok, err := ReadCurrent(root, session)
	if err != nil || !ok || current.Source.SHA256 != "bbb" {
		t.Fatalf("current=%+v ok=%v err=%v", current.Source, ok, err)
	}
}

func floatPtr(v float64) *float64 { return &v }
