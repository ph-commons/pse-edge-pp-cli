# Issue #57 — filings day evidence bundle

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

Agents need one Manila publication day's disclosure bodies and attachments
retained as original bytes, with a manifest. Search text and `disclosures
document` are not that bundle.

## Fix

- Add `filings day`. Discovery stays `announcements/search.ax`.
  `corpus_complete` is always false.
- Keep original bytes. Text is a sidecar. Revisions do not overwrite.
- Failures retry until `--max-attempts` when no bytes are retained.

## Files

- `internal/cli/filings_day.go`
- `internal/cli/filings_day_test.go`
- `internal/cli/disclosures_filings.go`
- `internal/cli/which.go`
- `internal/pseedge/disclosure_bytes.go`
- `internal/pseedge/disclosure_bytes_test.go`
- `README.md`, `CHANGELOG.md`, `SKILL.md`, `docs/downstream-integration.md`

## Verify

```
go test ./...
```
