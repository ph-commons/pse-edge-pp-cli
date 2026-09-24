# Issue #56 — Daily Quotation Report

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

Chart history from DisclosureCht.ax can omit the session the capture asked for.
The official Daily Quotation Report is a separate PDF. Downstream code was
about to scrape that PDF itself.

## Fix

- Add `quotation-report --date`. Discover the End of Day Quotes PDF from the
  market-report listing. Read the nonce and table id from that page.
- Keep the PDF, SHA-256, and discovery evidence under the data directory.
- Parse with `pdftotext -layout` into `pse-edge-quotation-report-v1`.
- A dash is null `reported_dash`. Do not write these rows into `pse_eod_prices`
  or change `pse-edge-export-eod-v1`.
- Same bytes do not add a revision. A new hash keeps the previous PDF.

## Files

- `internal/quotationreport/`
- `internal/cli/quotation_report.go`
- `internal/cli/quotation_report_test.go`
- `README.md`, `SKILL.md`, `CHANGELOG.md`, `docs/downstream-integration.md`

## Verify

```
go test ./internal/quotationreport/ ./internal/cli/
go vet ./...
```
