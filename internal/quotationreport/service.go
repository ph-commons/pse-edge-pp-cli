// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"path/filepath"
	"time"
)

// Open returns retained evidence for session. It does not call the network.
// ok is false when nothing has been admitted.
func Open(root string, session time.Time, symbols []string) (Report, bool, error) {
	day := session.Format("2006-01-02")
	if err := AlignQuotationCurrent(root, filepath.Join(root, "data.db"), day); err != nil {
		return Report{}, false, err
	}
	doc, found, err := ReadCurrent(root, day)
	if err != nil || !found {
		return Report{}, false, err
	}
	return reportFromDocument(doc, symbols, true, false, ""), true, nil
}

// Fetch discovers the session report, retains the PDF, and admits rows only
// after the internal date matches. Same bytes do not create a second revision.
func Fetch(ctx context.Context, root, listingURL string, session time.Time, symbols []string, now time.Time) (Report, error) {
	if now.IsZero() {
		now = time.Now()
	}
	day := session.Format("2006-01-02")
	found, findErr := FindReport(ctx, listingURL, session)
	dir := sessionDir(root, day)
	if findErr != nil {
		for _, pdf := range found.PDFs {
			_, _ = retainPDF(dir, pdf)
		}
		_ = writeFailure(dir, failureRecord{
			Status: statusOf(findErr),
			Detail: findErr.Error(),
			At:     now.UTC().Format(time.RFC3339),
		})
		rep := failedReport(day, findErr)
		if doc, ok, err := ReadCurrent(root, day); err == nil && ok {
			rep.RetainedSHA = doc.Source.SHA256
		}
		return rep, findErr
	}
	pdf := found.PDFs[0]
	path, err := retainPDF(dir, pdf)
	if err != nil {
		return failedReport(day, err), err
	}
	if doc, foundDoc, err := ReadCurrent(root, day); err != nil {
		return failedReport(day, err), err
	} else if foundDoc && doc.Source.SHA256 == pdf.SHA {
		if err := AlignQuotationCurrent(root, filepath.Join(root, "data.db"), day); err != nil {
			return failedReport(day, err), err
		}
		rep := reportFromDocument(doc, symbols, true, false, "")
		return rep, statusOrNil(rep.Status, "")
	}
	text, err := ExtractLayout(ctx, path)
	if err != nil {
		_ = writeFailure(dir, failureRecord{Status: statusOf(err), Detail: err.Error(), At: now.UTC().Format(time.RFC3339), SHA: pdf.SHA, URL: pdf.URL})
		rep := failedReport(day, err)
		rep.RetainedSHA = pdf.SHA
		return rep, err
	}
	parsed, err := ParseLayout(text, session, pdf.SHA)
	if err != nil {
		_ = writeFailure(dir, failureRecord{Status: statusOf(err), Detail: err.Error(), At: now.UTC().Format(time.RFC3339), SHA: pdf.SHA, URL: pdf.URL})
		rep := failedReport(day, err)
		rep.RetainedSHA = pdf.SHA
		return rep, err
	}
	for i := range parsed.Rows {
		parsed.Rows[i].SourceSHA256 = pdf.SHA
	}
	doc, revision, previous, err := Admit(root, day, pdf, found.Discovery, &parsed, now)
	if err != nil {
		return failedReport(day, err), err
	}
	rep := reportFromDocument(doc, symbols, false, revision, previous)
	return rep, statusOrNil(rep.Status, "")
}

func reportFromDocument(doc Document, symbols []string, reused, revision bool, previous string) Report {
	parsed := Parsed{SessionDate: doc.SessionDate, Rows: doc.Rows, Ambiguous: doc.Ambiguous}
	cov, rows, status := Cover(parsed, symbols)
	detail := ""
	if status == StatusMissingSymbols {
		detail = "one or more requested symbols are absent"
	}
	if status == StatusAmbiguousIdentity {
		detail = "one or more symbols were ambiguous and were not admitted"
	}
	src := doc.Source
	return Report{
		Contract:    ContractID,
		SessionDate: doc.SessionDate,
		Status:      status,
		Detail:      detail,
		Reused:      reused,
		Revision:    revision,
		PreviousSHA: previous,
		Source:      &src,
		Coverage:    cov,
		Rows:        rows,
		Ambiguous:   doc.Ambiguous,
		Limits:      defaultLimits(),
	}
}

func failedReport(day string, err error) Report {
	return Report{
		Contract:    ContractID,
		SessionDate: day,
		Status:      statusOf(err),
		Detail:      err.Error(),
		Rows:        []Row{},
		Ambiguous:   []AmbiguousSymbol{},
		Limits:      defaultLimits(),
	}
}

func statusOf(err error) string {
	if se, ok := err.(*Error); ok && se.Status != "" {
		return se.Status
	}
	return StatusAcquisitionFailed
}

func statusOrNil(status, _ string) error {
	switch status {
	case StatusOK:
		return nil
	case StatusMissingSymbols:
		return statusErr(status, "one or more requested symbols are absent")
	case StatusAmbiguousIdentity:
		return statusErr(status, "one or more symbols were ambiguous and were not admitted")
	default:
		return statusErr(status, status)
	}
}
