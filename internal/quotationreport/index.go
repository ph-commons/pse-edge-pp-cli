// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ph-commons/pse-edge-pp-cli/internal/store"
)

var sessionDirName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var quotationMeasures = []string{"bid", "ask", "open", "high", "low", "close", "volume", "value", "net_foreign"}

// IndexHit is one revision written by IndexRetained.
type IndexHit struct {
	SessionDate string `json:"session_date"`
	SHA256      string `json:"sha256"`
	Rows        int    `json:"rows"`
}

// IndexSkip is a JSON file that was not inserted.
type IndexSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// IndexResult is the retained-JSON pass. It does not download PDFs.
type IndexResult struct {
	Indexed []IndexHit  `json:"indexed"`
	Skipped []IndexSkip `json:"skipped"`
}

// SessionDirs lists quotation-report session directories. A non-empty session
// names that one directory. An empty session lists every YYYY-MM-DD child.
func SessionDirs(root, session string) ([]string, error) {
	if session != "" {
		if !sessionDirName.MatchString(session) {
			return nil, os.ErrInvalid
		}
		return []string{session}, nil
	}
	parent := filepath.Join(root, "quotation-reports")
	entries, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, ent := range entries {
		if ent.IsDir() && sessionDirName.MatchString(ent.Name()) {
			out = append(out, ent.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// IndexRetained loads revisions named by index.json. It does not read PDF bytes
// and it does not call the network. An existing database is also opened when
// the current file is skipped, so a stale is_current flag can be cleared.
func IndexRetained(ctx context.Context, root, dbPath, session string) (IndexResult, error) {
	result := IndexResult{Indexed: []IndexHit{}, Skipped: []IndexSkip{}}
	sessions, err := SessionDirs(root, session)
	if err != nil {
		return result, err
	}
	var db *store.Store
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()
	open := func() (*store.Store, error) {
		if db != nil {
			return db, nil
		}
		if dbPath == "" {
			return nil, os.ErrInvalid
		}
		opened, err := store.OpenWithContext(ctx, dbPath)
		if err != nil {
			return nil, err
		}
		db = opened
		return db, nil
	}
	for _, day := range sessions {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := indexSession(ctx, root, day, dbPath, &result, open); err != nil {
			return result, err
		}
	}
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Path < result.Skipped[j].Path })
	return result, nil
}

func indexSession(ctx context.Context, root, day, dbPath string, result *IndexResult, open func() (*store.Store, error)) error {
	dir := sessionDir(root, day)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	idx, err := loadIndex(dir)
	if err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, rev := range idx.Revisions {
		listed[rev.SHA] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		names = append(names, ent.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".json") || name == "index.json" {
			continue
		}
		stem := strings.TrimSuffix(name, ".json")
		if listed[stem] {
			continue
		}
		result.Skipped = append(result.Skipped, IndexSkip{Path: relReport(day, name), Reason: "not_listed"})
	}
	order := make([]revision, 0, len(idx.Revisions))
	var current *revision
	for i := range idx.Revisions {
		rev := idx.Revisions[i]
		if rev.SHA == idx.CurrentSHA {
			current = &rev
			continue
		}
		order = append(order, rev)
	}
	if current != nil {
		order = append(order, *current)
	}
	for _, rev := range order {
		name := rev.SHA + ".json"
		doc, err := loadDocument(dir, rev.SHA)
		if err != nil {
			reason := "missing"
			if !os.IsNotExist(err) {
				reason = "malformed"
			}
			result.Skipped = append(result.Skipped, IndexSkip{Path: relReport(day, name), Reason: reason})
			continue
		}
		if reason := rejectDocument(doc, day, rev.SHA); reason != "" {
			result.Skipped = append(result.Skipped, IndexSkip{Path: relReport(day, name), Reason: reason})
			continue
		}
		db, err := open()
		if err != nil {
			return err
		}
		if err := db.StoreQuotationRevision(ctx, quotationInput(doc)); err != nil {
			return err
		}
		result.Indexed = append(result.Indexed, IndexHit{SessionDate: day, SHA256: rev.SHA, Rows: len(doc.Rows)})
	}
	return finishSessionCurrent(ctx, day, dbPath, idx.CurrentSHA, result, open)
}

// finishSessionCurrent promotes the index.json current SHA only when that file
// was admitted. A skipped current file clears is_current so an older revision
// is not labeled current.
func finishSessionCurrent(ctx context.Context, day, dbPath, currentSHA string, result *IndexResult, open func() (*store.Store, error)) error {
	saw := false
	currentOK := false
	for _, hit := range result.Indexed {
		if hit.SessionDate != day {
			continue
		}
		saw = true
		if currentSHA != "" && hit.SHA256 == currentSHA {
			currentOK = true
		}
	}
	if currentSHA != "" && currentOK {
		db, err := open()
		if err != nil {
			return err
		}
		return db.PromoteQuotationRevision(ctx, day, currentSHA)
	}
	if !saw && currentSHA == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	db, err := open()
	if err != nil {
		return err
	}
	return db.ClearQuotationCurrent(ctx, day)
}

func rejectDocument(doc Document, day, sha string) string {
	if doc.Contract != ContractID {
		return "wrong_contract"
	}
	if doc.SessionDate != day {
		return "session_mismatch"
	}
	if doc.Source.SHA256 != sha {
		return "sha_mismatch"
	}
	for _, row := range doc.Rows {
		if rowContradicts(row) {
			return "contradictory_null"
		}
	}
	return ""
}

func rowContradicts(row Row) bool {
	status := row.FieldStatus
	for _, name := range quotationMeasures {
		ptr := measurePtr(row, name)
		got := ""
		if status != nil {
			got = status[name]
		}
		if ptr == nil && got == "" {
			return true
		}
		if got == FieldReportedDash && ptr != nil {
			return true
		}
	}
	return false
}

func measurePtr(row Row, name string) *float64 {
	switch name {
	case "bid":
		return row.Bid
	case "ask":
		return row.Ask
	case "open":
		return row.Open
	case "high":
		return row.High
	case "low":
		return row.Low
	case "close":
		return row.Close
	case "volume":
		return row.Volume
	case "value":
		return row.Value
	case "net_foreign":
		return row.NetForeign
	default:
		return nil
	}
}

func relReport(day, name string) string {
	return filepath.ToSlash(filepath.Join("quotation-reports", day, name))
}
