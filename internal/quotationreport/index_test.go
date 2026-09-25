// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

func TestIndexRetainedSkipsAndKeepsCount(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data.db")
	day := "2026-09-24"
	dir := sessionDir(root, day)
	closeV := 19.98
	valid := sampleDoc(day, "abc", Row{Symbol: "AT", IssueName: "ATLAS", RowLocator: "r1", Page: 1, Close: &closeV})
	valid.Ambiguous = []AmbiguousSymbol{{Symbol: "DUP", Locators: []string{"p1", "p2"}}}
	bad := sampleDoc(day, "bad", Row{Symbol: "DHI", RowLocator: "r1", Page: 1})
	bad.Rows[0].FieldStatus = nil
	dashed := sampleDoc(day, "dash", Row{Symbol: "ZZ", RowLocator: "r1", Page: 1, Close: &closeV, FieldStatus: map[string]string{"close": FieldReportedDash}})
	wrong := sampleDoc(day, "wrong", Row{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &closeV})
	wrong.Contract = "not-the-contract"
	mismatch := sampleDoc("2026-09-25", "mmm", Row{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &closeV})
	shaBad := sampleDoc(day, "shabad", Row{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &closeV})
	shaBad.Source.SHA256 = "other"
	for _, doc := range []Document{valid, bad, dashed, wrong, mismatch} {
		if err := writeJSON(filepath.Join(dir, doc.Source.SHA256+".json"), doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(filepath.Join(dir, "shabad.json"), shaBad); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "last-error.json"), failureRecord{Status: "acquisition_failed"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "index.json"), indexFile{
		SessionDate: day,
		CurrentSHA:  "abc",
		Revisions: []revision{
			{SHA: "bad"}, {SHA: "dash"}, {SHA: "wrong"}, {SHA: "mmm"}, {SHA: "shabad"}, {SHA: "abc"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(dir, "abc.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pdfPath, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pdfPath, 0o644) })

	other := sessionDir(root, "2026-09-25")
	otherDoc := sampleDoc("2026-09-25", "zzz", Row{Symbol: "AT", RowLocator: "r1", Page: 1, Close: &closeV})
	if err := writeJSON(filepath.Join(other, "zzz.json"), otherDoc); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(other, "index.json"), indexFile{SessionDate: "2026-09-25", CurrentSHA: "zzz", Revisions: []revision{{SHA: "zzz"}}}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := IndexRetained(ctx, root, dbPath, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Indexed) != 1 || res.Indexed[0].SHA256 != "abc" || res.Indexed[0].Rows != 1 {
		t.Fatalf("indexed=%+v", res.Indexed)
	}
	reasons := map[string]string{}
	for _, skip := range res.Skipped {
		reasons[skip.Path] = skip.Reason
	}
	want := map[string]string{
		"quotation-reports/2026-09-24/last-error.json": "not_listed",
		"quotation-reports/2026-09-24/bad.json":        "contradictory_null",
		"quotation-reports/2026-09-24/dash.json":       "contradictory_null",
		"quotation-reports/2026-09-24/wrong.json":      "wrong_contract",
		"quotation-reports/2026-09-24/mmm.json":        "session_mismatch",
		"quotation-reports/2026-09-24/shabad.json":     "sha_mismatch",
	}
	for path, reason := range want {
		if reasons[path] != reason {
			t.Fatalf("%s reason=%q want %q skipped=%v", path, reasons[path], reason, res.Skipped)
		}
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertCount := func(query string, want int) {
		t.Helper()
		var n int
		if err := db.DB().QueryRow(query).Scan(&n); err != nil || n != want {
			t.Fatalf("%s count=%d err=%v", query, n, err)
		}
	}
	assertCount(`SELECT COUNT(*) FROM pse_quotation_rows`, 1)
	assertCount(`SELECT COUNT(*) FROM pse_quotation_rows WHERE symbol='DUP'`, 0)
	assertCount(`SELECT COUNT(*) FROM pse_quotation_ambiguous WHERE symbol='DUP'`, 1)
	assertCount(`SELECT COUNT(*) FROM pse_quotation_revisions WHERE source_sha256='zzz'`, 0)

	again, err := IndexRetained(ctx, root, dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Indexed) != 2 {
		t.Fatalf("reindex=%+v", again.Indexed)
	}
	assertCount(`SELECT COUNT(*) FROM pse_quotation_rows WHERE source_sha256='abc'`, 1)
	assertCount(`SELECT COUNT(*) FROM pse_quotation_rows`, 2)
	third, err := IndexRetained(ctx, root, dbPath, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Indexed) != 1 {
		t.Fatalf("third=%+v", third.Indexed)
	}
	assertCount(`SELECT COUNT(*) FROM pse_quotation_rows WHERE source_sha256='abc'`, 1)
}

func sampleDoc(day, sha string, row Row) Document {
	row.SessionDate = day
	row.SourceSHA256 = sha
	row.SourceType = SourceType
	row = fillStatus(row)
	return Document{
		Contract:    ContractID,
		SessionDate: day,
		Source: Source{
			Type: SourceType, URL: "https://documents.pse.com.ph/" + sha + ".pdf", SHA256: sha,
			ByteLength: 4, AcquiredAt: "2026-09-24T12:00:00Z", PDFPath: sha + ".pdf",
		},
		Rows:      []Row{row},
		Ambiguous: []AmbiguousSymbol{},
	}
}

func fillStatus(row Row) Row {
	if row.FieldStatus == nil {
		row.FieldStatus = map[string]string{}
	}
	for _, name := range quotationMeasures {
		if _, ok := row.FieldStatus[name]; ok {
			continue
		}
		if measurePtr(row, name) == nil {
			row.FieldStatus[name] = FieldReportedDash
		} else {
			row.FieldStatus[name] = FieldOK
		}
	}
	return row
}
