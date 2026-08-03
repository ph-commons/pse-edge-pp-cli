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
