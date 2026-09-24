# Issue #53 — index snapshot sync gate + export availability (breadth freeze)

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

`syncMarketIndex` persisted the composite index reading (the local store's only
breadth source) only when the local clock was `post-close` **and** the page's
PSEI trade date exactly equalled the calendar's last completed session. Intraday
and next-morning runs skipped it, so `breadth` and the local `export index`
froze at the last post-close run. `export index` also emitted `change` and the
breadth fields as bare nulls, with no way to tell a field a row does not carry
from an unsynced session.

## Fix

- `internal/cli/sync_market.go`: `compositeSnapshotDate` treats the page's own
  PSEI trade stamp as the finality signal — store when `pageDate <=
  lastCompleted`, under the page's own date; a later stamp is the in-progress
  session and is skipped. `compositeIndexMismatch` skips a page whose indices
  disagree on the session date (`composite_inconsistent`) rather than misdating
  it, and a completed-but-lagging page emits `composite_lagging`. The undated
  breadth summary is attached to the PSEI row only when every index shares the
  session.
- `internal/cli/breadth.go`: `breadthWindowNote` states when a requested window
  has no breadth-bearing sessions and names the sync path.
- `internal/store/pse_export.go`: `ExportIndexRow` gains `change_status` and
  `breadth_status` (`ok` | `unavailable`), additive on `pse-edge-export-index-v1`.
- `docs/downstream-integration.md`: index contract row plus the availability and
  session-alignment rule.
- `CHANGELOG.md`: Unreleased / Fixed bullet.

## Files

- `internal/cli/sync_market.go`
- `internal/cli/breadth.go`
- `internal/store/pse_export.go`
- `internal/cli/sync_market_test.go`
- `internal/cli/breadth_test.go`
- `internal/store/pse_export_test.go`
- `docs/downstream-integration.md`
- `CHANGELOG.md`

## Verify

```bash
go test ./...
go vet ./...
```
