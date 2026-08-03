# Issue #8 — change-cell regex (down-day / NBSP / latent sign inversion)

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

`changeCellRE` in `internal/pseedge/pseedge.go` failed on PSE Edge stockData.do
down-day closes (observed 2026-07-27, reporter jiegomojiica):

1. Percent group rejected interior whitespace in `( 1.32%)`.
2. Optional `(up|down)?` plus ASCII-only `\s` / literal `&nbsp;` could not
   bind a U+00A0-separated prefix; unanchored match from the digits left
   group 1 empty → **silent positive change** if (1) were fixed alone.

## Fix

- `normalizeChangeCell`: Unescape entities, map NBSP (and common entity
  leftovers) to ASCII space before matching.
- Require `(up|down)` prefix; allow `\s*` inside the percent parentheses.
- Case-insensitive direction word; regression fixtures for live NBSP +
  ASCII spaced percent forms.

## Files

- `internal/pseedge/pseedge.go`
- `internal/pseedge/pseedge_test.go`

## Verify

```bash
go test ./internal/pseedge/ -count=1 -run 'ParseStockData'
go test ./...
```
