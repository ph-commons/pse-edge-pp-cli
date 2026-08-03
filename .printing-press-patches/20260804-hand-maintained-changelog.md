# Hand-maintained CHANGELOG (independent fork)

## Policy (2026-08-04)

`CHANGELOG.md` is **hand-maintained** Keep a Changelog for this public repo
(tags + goreleaser). Printing-press-library’s “do not hand-edit release sections”
applies to **library catalog** publishes only — not this fork.

## Agent / PR contract

1. User-facing fix/feature → bullet under `## [Unreleased]` in the same PR.
2. On tag release → promote Unreleased to `## [x.y.z] - YYYY-MM-DD`.
3. Do not hand-bump runtime `var version`; tags/ldflags own the version string.

## Files

- `CHANGELOG.md` — full policy preamble + Unreleased
- `AGENTS.md` — Release Ledger section rewritten for independent release
