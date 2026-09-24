// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type indexFile struct {
	SessionDate string     `json:"session_date"`
	CurrentSHA  string     `json:"current_sha256"`
	Revisions   []revision `json:"revisions"`
}

type revision struct {
	SHA          string `json:"sha256"`
	URL          string `json:"source_url"`
	AcquiredAt   string `json:"acquired_at"`
	UploadDate   string `json:"upload_date"`
	SessionTitle string `json:"document_session_date"`
}

type failureRecord struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
	At     string `json:"at"`
	SHA    string `json:"sha256,omitempty"`
	URL    string `json:"url,omitempty"`
}

func sessionDir(root, session string) string {
	return filepath.Join(root, "quotation-reports", session)
}

func loadIndex(dir string) (indexFile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return indexFile{}, nil
		}
		return indexFile{}, err
	}
	var idx indexFile
	if err := json.Unmarshal(raw, &idx); err != nil {
		return indexFile{}, err
	}
	return idx, nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func writeFailure(dir string, rec failureRecord) error {
	return writeJSON(filepath.Join(dir, "last-error.json"), rec)
}

func retainPDF(dir string, pdf PDF) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, pdf.SHA+".pdf")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.WriteFile(path, pdf.Body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func loadDocument(dir, sha string) (Document, error) {
	var doc Document
	raw, err := os.ReadFile(filepath.Join(dir, sha+".json"))
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

// ReadCurrent returns the admitted document for a session, if one exists.
func ReadCurrent(root, session string) (Document, bool, error) {
	dir := sessionDir(root, session)
	idx, err := loadIndex(dir)
	if err != nil {
		return Document{}, false, err
	}
	if idx.CurrentSHA == "" {
		return Document{}, false, nil
	}
	doc, err := loadDocument(dir, idx.CurrentSHA)
	if err != nil {
		return Document{}, false, err
	}
	if doc.Source.PDFPath == "" {
		doc.Source.PDFPath = filepath.Join(dir, idx.CurrentSHA+".pdf")
	}
	return doc, true, nil
}

// Admit stores PDF bytes and, when parsed is non-nil, makes that hash current.
// The same hash does not add another revision. A new hash keeps the previous PDF.
func Admit(root, session string, pdf PDF, discovery Discovery, parsed *Parsed, now time.Time) (Document, bool, string, error) {
	dir := sessionDir(root, session)
	path, err := retainPDF(dir, pdf)
	if err != nil {
		return Document{}, false, "", err
	}
	idx, err := loadIndex(dir)
	if err != nil {
		return Document{}, false, "", err
	}
	if idx.SessionDate == "" {
		idx.SessionDate = session
	}
	previous := idx.CurrentSHA
	if parsed == nil {
		return Document{}, false, previous, nil
	}
	if previous == pdf.SHA {
		doc, err := loadDocument(dir, pdf.SHA)
		if err == nil && doc.Contract == ContractID {
			doc.Source.PDFPath = path
			return doc, false, "", nil
		}
	}
	doc := Document{
		Contract:    ContractID,
		SessionDate: session,
		Source: Source{
			Type:       SourceType,
			URL:        pdf.URL,
			SHA256:     pdf.SHA,
			ByteLength: len(pdf.Body),
			AcquiredAt: now.UTC().Format(time.RFC3339),
			Discovery:  discovery,
			PDFPath:    path,
		},
		Rows:      parsed.Rows,
		Ambiguous: parsed.Ambiguous,
	}
	if err := writeJSON(filepath.Join(dir, pdf.SHA+".json"), doc); err != nil {
		return Document{}, false, previous, err
	}
	replaced := previous != "" && previous != pdf.SHA
	if previous != pdf.SHA {
		idx.Revisions = append(idx.Revisions, revision{
			SHA:          pdf.SHA,
			URL:          pdf.URL,
			AcquiredAt:   doc.Source.AcquiredAt,
			UploadDate:   discovery.UploadDate,
			SessionTitle: discovery.SessionTitle,
		})
		idx.CurrentSHA = pdf.SHA
		if err := writeJSON(filepath.Join(dir, "index.json"), idx); err != nil {
			return Document{}, false, previous, err
		}
	}
	if replaced {
		return doc, true, previous, nil
	}
	return doc, false, "", nil
}
