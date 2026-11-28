# PSE Edge Printed CLI Agent Guide

This directory is a generated `pse-edge-pp-cli` printed CLI. It was produced by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press), so treat systemic fixes as upstream Printing Press fixes first. Keep local edits narrow and document why a generated-tree patch belongs here.

## Local Operating Contract

For routine agent use, disable local learning before the first invocation, then
ask the generated CLI for current runtime truth:

```bash
export PSE_EDGE_NO_LEARN=true
pse-edge-pp-cli doctor --json
pse-edge-pp-cli agent-context --pretty
```

Use runtime discovery instead of relying on a copied command list:

```bash
pse-edge-pp-cli which "<capability>" --json
pse-edge-pp-cli <command> --help
```

Add `--agent` to command invocations for JSON, compact output, non-interactive defaults, no color, and confirmation-safe scripting:

```bash
pse-edge-pp-cli <command> --agent
```

Before running an unfamiliar command that may mutate remote state, inspect its help and prefer a dry run:

```bash
pse-edge-pp-cli <command> --help
pse-edge-pp-cli <command> --dry-run --agent
```

Use `--yes --no-input` only after the target, arguments, and side effects are clear.

## Optional local learning

For MCP, set `PSE_EDGE_NO_LEARN=true` in the server's environment. The per-command
alternative is `--no-learn`. These switches disable learning, not market-data
storage. Without them the CLI still captures learning state automatically.

Use local recall, teach, playbooks and candidate updates only when explicitly
requested and permitted by the workspace's memory policy. Follow its designated
memory owner; CLI guidance grants no memory-write authority. Do not teach in the
background after routine queries. Preserve existing learning data.

When local learning is authorized, candidates remain unverified: check a proposed
trial is in scope, verify its result, then confirm or reject it. Do not teach a
duplicate candidate. Strip names, emails, phone numbers and account IDs from
queries and notes. Read warnings and fetch current data rather than treating a
remembered mapping as current market evidence.

The store's schema stamp is one-way: opening it with a newer binary can prevent
older binaries from using it. See the README upgrade note.

For install, examples, and longer product guidance, read `README.md` and `SKILL.md`. This file intentionally stays small so repo-local agents get invariant local guidance without duplicating the generated docs.

## Release Ledger

This repo is an **independent** public CLI (tags + goreleaser), not a library-catalog
print that waits on `mvanhorn/printing-press-library` post-merge stamping.

- **`CHANGELOG.md` is hand-maintained** (Keep a Changelog). Every user-facing
  fix/feature PR **must** add a bullet under `## [Unreleased]`. On tag release,
  promote Unreleased into `## [x.y.z] - YYYY-MM-DD` and clear Unreleased.
- Do **not** hand-edit runtime `var version = ...` for release bookkeeping —
  version comes from ldflags / git tag / `resolveVersion`.
- `.printing-press-release.json` may remain from the original print; preserve it
  on reprint if present, but it is **not** the SSOT for this fork’s release notes.
- `.printing-press-patches/` remains the reprint-guard for code/doc intent
  (altitude: durable guard, not a substitute for CHANGELOG).

## Local Customizations

This directory is **generated output** -- a fresh print can overwrite the whole tree, so ad-hoc hand-edits don't survive on their own. If you modify the generated code, record each change under `.printing-press-patches/` (parallel to `.printing-press.json`) so a regen carries the intent forward instead of silently dropping it.

The entry shape, and the altitude to write it at -- a durable reprint-guard, not a changelog -- live in the public library's `AGENTS.md`, which is the single source of truth; this guide intentionally doesn't duplicate them.
