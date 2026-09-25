# Issue #61 — Quotation rows in SQLite

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

Admitted Daily Quotation Report rows lived only in per-session JSON. A later
session could not be queried from the CLI store. Chart history cannot hold a
SHA, field status, net foreign, page, or revision.

## Fix

- Add three lazy tables in the existing `data.db`: `pse_quotation_revisions`,
  `pse_quotation_rows`, and `pse_quotation_ambiguous`. Do not add them to
  `EnsurePSEEdgeTables` and do not bump `StoreSchemaVersion`.
- `Admit` writes those rows in one transaction before it updates `index.json`.
- `quotation-report query` serves `pse-edge-quotation-rows-v1`. A dash stays
  null. `--sha` can return a retained previous revision.
- `quotation-report index` reads retained JSON named by `index.json`. It does
  not download PDFs.
- Leave chart history alone. Do not write these rows into `pse_eod_prices`
  or change `pse-edge-export-eod-v1`.

## Files

- `internal/store/quotation_report.go`
- `internal/store/quotation_report_test.go`
- `internal/quotationreport/store.go`
- `internal/quotationreport/doc.go`
- `internal/quotationreport/store_test.go`
- `internal/quotationreport/index.go`
- `internal/quotationreport/index_test.go`
- `internal/cli/quotation_report.go`
- `internal/cli/quotation_report_query.go`
- `internal/cli/quotation_report_query_test.go`
- `README.md`, `SKILL.md`, `CHANGELOG.md`, `docs/downstream-integration.md`

## Verify

```
go test ./...
```
