# Issue #52 — direction-only change cell (edge leg dropped, phisix change_pct 0)

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

EDGE can serve the `Change(% Change)` cell as the direction word with an
empty magnitude, e.g. `down (%)`. `changeCellRE` required a numeric
magnitude, so the cell matched no pattern and `ParseStockData` returned a
`*MarkupDriftError`. `quote` then dropped the whole edge leg and served
phisix only — where a null `change` could be paired with `change_pct: 0`,
reading as a real "unchanged" result.

## Fix

- `changeCellRE` accepts a direction-only alternative (`(up|down)\s*\(\s*%\s*\)`)
  alongside the existing magnitude form.
- `ParseStockData` sets `Snapshot.ChangeMagnitudeMissing` (not serialized) for
  a direction-only cell and leaves `Change`/`PctChange` nil.
- `quote` no longer treats `ChangeMagnitudeMissing` as a closed session, and
  `enforceChangePair` clears a numeric `change_pct` whenever `change` is null.

## Files

- `internal/pseedge/pseedge.go`
- `internal/pseedge/pseedge_test.go`
- `internal/cli/quote.go`
- `internal/cli/quote_test.go`

## Verify

```bash
go test ./internal/pseedge/ -count=1 -run ParseStockData
go test ./...
```

## Changelog

Entry under `CHANGELOG.md` → `## [Unreleased]` / Fixed (policy: hand-maintained
for this independent repo as of 2026-08-04).
