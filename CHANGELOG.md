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

### Fixed

- Parse PSE Edge `stockData.do` down-day `Change(% Change)` cells that use U+00A0
  and interior percent whitespace; require `up`/`down` prefix so an unmatched
  direction word cannot silently invert sign ([#8](https://github.com/ngpestelos/pse-edge-pp-cli/issues/8)).

## [0.1.1] - 2026-07-27

### Changed

- Five-step densify and parallel quote path; `resolveVersion` for non-dev
  installs (see release notes / git history).

## [0.1.0] - 2026-07-27

### Added

- Initial public release: agent-native PSE Edge CLI (quotes, filings, local
  history, MCP).
