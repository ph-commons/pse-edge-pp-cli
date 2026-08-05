# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Policy (2026-08-04):** Hand-maintained. Every user-facing fix/feature PR updates
`## [Unreleased]`. On tag release, move those bullets under a new version heading
and clear Unreleased. Do **not** leave changelog work to printing-press-library
automation for this independent repo (that rule applies only when publishing
*into* the library catalog).

## [Unreleased]

### Security

- `scripts/install.sh` verifies prebuilt release tarballs against the release `checksums.txt` (SHA-256) before extract; refuse install on missing entry or mismatch ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).
- `filings` / `filings get` HTTP clients set an explicit 60s timeout ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).
- MCP HTTP default bind is loopback-only (`127.0.0.1:7777`); warn on non-loopback binds (no auth) ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).
- SQLite DSN builder rejects path URI metacharacters that could override `mode=ro`; MCP blocks `--db` ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).
- Cross-host HTTP redirects drop `Config.Headers` keys, not only `Authorization` ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).
- Security review notes: `docs/security-review-20260805.md` ([#13](https://github.com/ngpestelos/pse-edge-pp-cli/issues/13)).


### Added

- Local-store export resources for downstream pipelines: `export eod`, `export index`, `export companies-local` with versioned per-row `contract` ids (`pse-edge-export-*-v1`) ([#9](https://github.com/ngpestelos/pse-edge-pp-cli/issues/9)).
- Downstream integration guide: `docs/downstream-integration.md` ([#9](https://github.com/ngpestelos/pse-edge-pp-cli/issues/9)).
- `filings get --edge-no` — direct `openDiscViewer.do` lookup for a known disclosure hash when search omits it ([#10](https://github.com/ngpestelos/pse-edge-pp-cli/issues/10)).

### Changed

- `filings` JSON now exposes search telemetry and honesty fields: `returned_count`, `from_date`, `to_date`, `company_id`, `limit`, `max_scan_pages`, `truncated`, `page_cap_hit`, `complete` (relative to the search result set only), `newest_disclosed_at` / `oldest_disclosed_at`, `freshness_gap_days`, and standing `warnings` that search is not an authoritative complete corpus ([#10](https://github.com/ngpestelos/pse-edge-pp-cli/issues/10)).

## [0.1.2] - 2026-08-04

### Fixed

- Parse PSE Edge `stockData.do` down-day `Change(% Change)` cells that use U+00A0
  and interior percent whitespace; require `up`/`down` prefix so an unmatched
  direction word cannot silently invert sign
  ([#8](https://github.com/ngpestelos/pse-edge-pp-cli/issues/8),
  [#14](https://github.com/ngpestelos/pse-edge-pp-cli/pull/14)).

### Changed

- Bump `golang.org/x/net` 0.55.0 → 0.57.0
  ([#12](https://github.com/ngpestelos/pse-edge-pp-cli/pull/12)).

## [0.1.1] - 2026-07-27

### Changed

- Five-step densify and parallel quote path; `resolveVersion` for non-dev
  installs (see release notes / git history).

## [0.1.0] - 2026-07-27

### Added

- Initial public release: agent-native PSE Edge CLI (quotes, filings, local
  history, MCP).

[Unreleased]: https://github.com/ngpestelos/pse-edge-pp-cli/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/ngpestelos/pse-edge-pp-cli/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/ngpestelos/pse-edge-pp-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ngpestelos/pse-edge-pp-cli/releases/tag/v0.1.0
