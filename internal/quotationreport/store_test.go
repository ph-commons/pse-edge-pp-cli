// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
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

func TestAdmitNilParsedSkipsSQLite(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	pdf := PDF{URL: "https://documents.pse.com.ph/a.pdf", Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, Discovery{}, nil, time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "data.db")); !os.IsNotExist(err) {
		t.Fatalf("data.db exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionDir(root, session), "aaa.json")); !os.IsNotExist(err) {
		t.Fatalf("json exists: %v", err)
	}
}

func TestAdmitPersistsRowsAndKeepsPreviousSHA(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf", UploadDate: "September 24, 2026", SessionTitle: "September 24, 2026"}
	parsed := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, parsed, when); err != nil {
		t.Fatal(err)
	}
	parsed.Rows[0].Close = floatPtr(2)
	if _, revision, _, err := Admit(root, session, pdf, disc, parsed, when.Add(time.Hour)); err != nil {
		t.Fatal(err)
	} else if revision {
		t.Fatal("same sha appended a revision")
	}
	idx, err := loadIndex(sessionDir(root, session))
	if err != nil || len(idx.Revisions) != 1 || idx.CurrentSHA != "aaa" {
		t.Fatalf("index=%+v err=%v", idx, err)
	}
	db, err := store.Open(filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	got, err := db.QueryQuotationRevisions(ctx, store.QuotationQuery{From: session, To: session, SHA: "aaa"})
	if err != nil || len(got) != 1 || got[0].Rows[0].Close == nil || *got[0].Rows[0].Close != 2 {
		t.Fatalf("replaced=%+v err=%v", got, err)
	}
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	second := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(3)}}}
	if _, revision, previous, err := Admit(root, session, next, disc, second, when.Add(2*time.Hour)); err != nil || !revision || previous != "aaa" {
		t.Fatalf("revision=%v previous=%s err=%v", revision, previous, err)
	}
	old, err := db.QueryQuotationRevisions(ctx, store.QuotationQuery{From: session, To: session, SHA: "aaa"})
	if err != nil || len(old) != 1 || old[0].Current || *old[0].Rows[0].Close != 2 {
		t.Fatalf("old=%+v err=%v", old, err)
	}
	cur, err := db.QueryQuotationRevisions(ctx, store.QuotationQuery{From: session, To: session})
	if err != nil || len(cur) != 1 || cur[0].SourceSHA256 != "bbb" || *cur[0].Rows[0].Close != 3 {
		t.Fatalf("current=%+v err=%v", cur, err)
	}
}

func TestAdmitPersistErrorLeavesIndex(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf"}
	parsed := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, parsed, when); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "data.db")
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}
	next := PDF{URL: "https://documents.pse.com.ph/c.pdf", Body: []byte("%PDF-3\n"), SHA: "ccc"}
	if _, _, _, err := Admit(root, session, next, disc, parsed, when.Add(time.Hour)); err == nil {
		t.Fatal("expected persist error")
	}
	idx, err := loadIndex(sessionDir(root, session))
	if err != nil {
		t.Fatal(err)
	}
	if idx.CurrentSHA != "aaa" || len(idx.Revisions) != 1 {
		t.Fatalf("index=%+v", idx)
	}
}

func TestAdmitIndexWriteFailureKeepsPreviousCurrent(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf"}
	first := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, first, when); err != nil {
		t.Fatal(err)
	}
	prev := writeAdmittedIndex
	writeAdmittedIndex = func(string, any) error { return errors.New("index write failed") }
	t.Cleanup(func() { writeAdmittedIndex = prev })
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	second := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(2)}}}
	if _, _, _, err := Admit(root, session, next, disc, second, when.Add(time.Hour)); err == nil {
		t.Fatal("expected index error")
	}
	idx, err := loadIndex(sessionDir(root, session))
	if err != nil || idx.CurrentSHA != "aaa" {
		t.Fatalf("index=%+v err=%v", idx, err)
	}
	db, err := store.Open(filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cur, err := db.QueryQuotationRevisions(context.Background(), store.QuotationQuery{From: session, To: session})
	if err != nil || len(cur) != 1 || cur[0].SourceSHA256 != "aaa" {
		t.Fatalf("sqlite current=%+v err=%v", cur, err)
	}
	doc, ok, err := ReadCurrent(root, session)
	if err != nil || !ok || doc.Source.SHA256 != "aaa" {
		t.Fatalf("read=%s ok=%v err=%v", doc.Source.SHA256, ok, err)
	}
}

func TestOpenRepairsInterruptedPromote(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf"}
	first := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, first, when); err != nil {
		t.Fatal(err)
	}
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	second := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(2)}}}
	doc := Document{
		Contract: ContractID, SessionDate: session,
		Source: Source{Type: SourceType, URL: next.URL, SHA256: next.SHA, ByteLength: len(next.Body), AcquiredAt: when.Add(time.Hour).UTC().Format(time.RFC3339), PDFPath: next.SHA + ".pdf"},
		Rows:   []Row{fillStatus(second.Rows[0])},
	}
	dir := sessionDir(root, session)
	if err := writeJSON(filepath.Join(dir, next.SHA+".json"), doc); err != nil {
		t.Fatal(err)
	}
	if err := storeAdmitted(root, doc); err != nil {
		t.Fatal(err)
	}
	idx, err := loadIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx.CurrentSHA = next.SHA
	idx.Revisions = append(idx.Revisions, revision{SHA: next.SHA, URL: next.URL})
	if err := writeJSON(filepath.Join(dir, "index.json"), idx); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := db.QueryQuotationRevisions(context.Background(), store.QuotationQuery{From: session, To: session})
	db.Close()
	if err != nil || len(cur) != 1 || cur[0].SourceSHA256 != "aaa" {
		t.Fatalf("before repair=%+v err=%v", cur, err)
	}
	report, ok, err := Open(root, when, nil)
	if err != nil || !ok || report.Source == nil || report.Source.SHA256 != "bbb" {
		t.Fatalf("open=%+v ok=%v err=%v", report.Source, ok, err)
	}
	db, err = store.Open(filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cur, err = db.QueryQuotationRevisions(context.Background(), store.QuotationQuery{From: session, To: session})
	if err != nil || len(cur) != 1 || cur[0].SourceSHA256 != "bbb" || !cur[0].Current {
		t.Fatalf("after repair=%+v err=%v", cur, err)
	}
}

func TestAdmitPromoteFailureReportsRestoreError(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf"}
	parsed := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, parsed, when); err != nil {
		t.Fatal(err)
	}
	prevPromote := promoteAdmitted
	prevRestore := restoreAdmittedIndex
	promoteAdmitted = func(string, string, string) error { return errors.New("promote failed") }
	restoreAdmittedIndex = func(string, []byte, bool) error { return errors.New("restore failed") }
	t.Cleanup(func() {
		promoteAdmitted = prevPromote
		restoreAdmittedIndex = prevRestore
	})
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	_, _, _, err := Admit(root, session, next, disc, parsed, when.Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "restore index") {
		t.Fatalf("err=%v", err)
	}
}

func TestAdmitPromoteFailureRestoresIndex(t *testing.T) {
	root := t.TempDir()
	session := "2026-09-24"
	when := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	disc := Discovery{PDFURL: "https://documents.pse.com.ph/a.pdf"}
	first := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(1)}}}
	pdf := PDF{URL: disc.PDFURL, Body: []byte("%PDF-1\n"), SHA: "aaa"}
	if _, _, _, err := Admit(root, session, pdf, disc, first, when); err != nil {
		t.Fatal(err)
	}
	prev := promoteAdmitted
	promoteAdmitted = func(string, string, string) error { return errors.New("promote failed") }
	t.Cleanup(func() { promoteAdmitted = prev })
	next := PDF{URL: "https://documents.pse.com.ph/b.pdf", Body: []byte("%PDF-2\n"), SHA: "bbb"}
	second := &Parsed{SessionDate: session, Rows: []Row{{Symbol: "AT", RowLocator: "r1", Close: floatPtr(2)}}}
	if _, _, _, err := Admit(root, session, next, disc, second, when.Add(time.Hour)); err == nil {
		t.Fatal("expected promote error")
	}
	idx, err := loadIndex(sessionDir(root, session))
	if err != nil || idx.CurrentSHA != "aaa" || len(idx.Revisions) != 1 {
		t.Fatalf("index=%+v err=%v", idx, err)
	}
	db, err := store.Open(filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cur, err := db.QueryQuotationRevisions(context.Background(), store.QuotationQuery{From: session, To: session})
	if err != nil || len(cur) != 1 || cur[0].SourceSHA256 != "aaa" {
		t.Fatalf("sqlite current=%+v err=%v", cur, err)
	}
}

func floatPtr(v float64) *float64 { return &v }
