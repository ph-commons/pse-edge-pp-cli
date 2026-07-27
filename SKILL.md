---
name: pp-pse-edge
description: "Agent-native Philippine Stock Exchange CLI — quotes, filings, and a local price history no free API serves. Trigger phrases: `quote AT`, `how is the PSEi doing`, `PSE market breadth`, `check PSE disclosures for GTCAP`, `has the 17-Q been filed`, `PSE movers this week`, `use pse-edge`, `run pse-edge`."
version: "1.1.0"
author: "Nestor G Pestelos Jr"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  hermes:
    tags: [PSE, Philippine Stock Exchange, EDGE, CLI, finance]
    related_skills: [pse-edge, pse-edge-chrome, pse-data-sourcing, investor-vault-operations, ship-go-cli-fleet]
  openclaw:
    requires:
      bins:
        - pse-edge-pp-cli
    install:
      - kind: go
        bins: [pse-edge-pp-cli]
        module: github.com/ngpestelos/pse-edge-pp-cli/cmd/pse-edge-pp-cli
---

# PSE Edge — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `pse-edge-pp-cli` binary. **Verify it is installed before any command.**

```bash
which pse-edge-pp-cli || true
pse-edge-pp-cli --version
```

Missing or **stale** binary (current release **v0.1.0**+):

1. Prefer prebuilt: `curl -fsSL https://raw.githubusercontent.com/ngpestelos/pse-edge-pp-cli/main/scripts/install.sh | bash` (upgrades even if binary exists).
2. Else Go ≥1.26.5: `go install github.com/ngpestelos/pse-edge-pp-cli/cmd/pse-edge-pp-cli@latest`
3. Ensure `$GOPATH/bin`, `$HOME/go/bin`, or `~/.local/bin` is on `PATH`.

**Do not run skill commands until `--version` works.**

Every figure carries its source and as-of trading date, so agents can quote the tape without narrating stale prices. Sync builds a local SQLite registry and EOD history (backfillable to 2021 for the PSEi), unlocking history, drift, breadth, and movers commands that no PSE endpoint can answer. Dual-sourced quotes survive either upstream dying.

## When to Use This CLI

Use this CLI for Philippine Stock Exchange questions: EOD quotes with honest as-of dates, PSEi and sector snapshots, market breadth, corporate disclosures and 17-Q filing status, and any question spanning history (drift, movers, breadth trends) via the local store. It is the right tool for agents that must never quote an undated price.

## Anti-triggers

Do not use this CLI for:
- Do not use for intraday prices, tick data, or VWAP — no free PSE source provides them and this CLI is EOD-only
- Do not use for placing or simulating trades — it is read-only market data, not a broker
- Do not use for non-Philippine exchanges
- Do not use its local history as an authoritative audit trail — the vault journal (where present) remains the authoritative record

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local history that compounds
- **`history`** — Query daily OHLC/value history for any ticker or the PSEi from the local store — data no free PSE API serves.

  _Reach for this when a question spans more than the current session; every row carries source and as-of trading date._

  ```bash
  pse-edge-pp-cli history AT --since 30d --agent
  ```
- **`drift`** — Percent change over a window, absolute and versus the PSEi over the same sessions, with 52-week band position.

  _Answers 'how has this done vs the market' in one call instead of two fetches and hand math._

  ```bash
  pse-edge-pp-cli drift AT --since 90d --agent
  ```
- **`breadth`** — Advance/decline/unchanged and value-traded trend over time from local index snapshots.

  _Breadth direction over weeks is the tape context a single day's number cannot give._

  ```bash
  pse-edge-pp-cli breadth --since 30d --agent
  ```
- **`movers`** — Ranked gainers and losers across the synced universe for a window, with universe size and as-of stated.

  _Weekend tape review in one call; the output names its universe so partial syncs cannot mislead._

  ```bash
  pse-edge-pp-cli movers --since 7d --agent
  ```

### As-of discipline
- **`session`** — Philippine trading-calendar verdict: trading day or not, pre/post-close gate, last completed trading date.

  _Gate every EOD figure on this before trusting it — blank change fields on non-trading days are states, not zeros._

  ```bash
  pse-edge-pp-cli session --json
  ```
- **`stale`** — Per-ticker last-synced trading date versus the last completed trading day, with in-series gaps listed.

  _Run before analytics so a silently lagged sync cannot masquerade as a market move._

  ```bash
  pse-edge-pp-cli stale --json
  ```

### Filing rhythm
- **`deadlines`** — Computed 17-Q/17-A due dates per SRC Rule 68 45/105-day windows, joined to local disclosures: filed, pending, or overdue.

  _Answers 'has the quarterly report landed and when is it due' without a browser search._

  ```bash
  pse-edge-pp-cli deadlines AT --json
  ```

## Command Reference

**companies** — Listed-company registry: directory, lookup, and profiles

- `pse-edge-pp-cli companies directory` — One page of the full listed-company directory (name, symbol, cmpy_id, security_id)
- `pse-edge-pp-cli companies lookup` — Search companies by name or symbol prefix (first 20 alphabetical matches only — use exact-match filtering)
- `pse-edge-pp-cli companies profile` — Company profile page: sector, subsector, incorporation, auditor

**disclosures** — Corporate disclosures: search, view, and read filing documents

- `pse-edge-pp-cli disclosures document` — Full disclosure document content as server-rendered HTML (use this, never the broken downloadFile.do PDF path)
- `pse-edge-pp-cli disclosures search` — Search disclosures by company, template, and date range (server-side
- `pse-edge-pp-cli disclosures view` — Disclosure viewer wrapper for one filing

**financials** — 17-Q/17-A financial summary tables (annual + quarterly)

- `pse-edge-pp-cli financials` — Balance-sheet and income-statement summary tables for a company (server-rendered HTML

**market** — Market-wide data: PSEi, sector indices, breadth, totals

- `pse-edge-pp-cli market` — Composite/sector page: PSEi and sector indices, market summary (volume, value, trades, advances/declines/unchanged)

**prices** — Per-ticker prices: current session snapshot and daily history

- `pse-edge-pp-cli prices history` — Daily OHLC and value-traded history for one security (first-party JSON)
- `pse-edge-pp-cli prices snapshot` — Current-session stock data page: last price, change, OHLC, volume, value, market cap

**quotes** — Fast EOD quotes via the community phisix JSON mirror (unofficial redeploy — may vanish; edge endpoints are the first-party fallback)

- `pse-edge-pp-cli quotes <symbol>` — EOD quote for one ticker: close, percent change, volume.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pse-edge-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Recipes

### Morning tape check

```bash
pse-edge-pp-cli market --json --select psei,breadth.advances,breadth.declines,breadth.unchanged,totals.value
```

One bounded call for index level and breadth without the full sector payload.

### Position review with as-of discipline

```bash
pse-edge-pp-cli quote AT GTCAP HTI --agent --select symbol,close,change_pct,as_of,stale,source
```

Narrow the quote payload to the fields an advisor actually cites.

### Quarter-end filing sweep

```bash
pse-edge-pp-cli deadlines AT --json
```

Computed 17-Q due dates joined to actual filings — filed, pending, or overdue.

### Disclosure sweep with client-side keyword

```bash
pse-edge-pp-cli filings GTCAP --from-date 01-01-2026 --json
```

Lists the year's disclosures and feeds the local index behind deadlines; --keyword is matched client-side because the endpoint ignores it.

### Relative strength question

```bash
pse-edge-pp-cli drift AT --since 90d --agent
```

Absolute and vs-PSEi performance with 52-week band position from the local store.

## Auth Setup

No authentication required.

Run `pse-edge-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  pse-edge-pp-cli disclosures search --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — safe to retry any command: every command is a read against public market data

### Response envelope

Generated API resource commands (companies, prices, market, quotes, disclosures, financials groups) wrap output in a provenance envelope; the novel local-store commands (history, breadth, movers, stale) emit bare JSON with inline source/as_of/stale fields:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set — piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `PSE_EDGE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `PSE_EDGE_CONFIG_DIR`, `PSE_EDGE_DATA_DIR`, `PSE_EDGE_STATE_DIR`, `PSE_EDGE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `PSE_EDGE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `data.db` (the local market store). `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files. This CLI has no credentials — the API is public.
- Run `pse-edge-pp-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "pse-edge": {
        "command": "pse-edge-pp-mcp",
        "env": {
          "PSE_EDGE_HOME": "/srv/pse-edge"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `PSE_EDGE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `PSE_EDGE_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
pse-edge-pp-cli recall "<user's question>" --agent
```

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "pse-edge-pp-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `pse-edge-pp-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `pse-edge-pp-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities — direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `pse-edge-pp-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
pse-edge-pp-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
pse-edge-pp-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
pse-edge-pp-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know — a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback — fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
pse-edge-pp-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`pse-edge-pp-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `PSE_EDGE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
pse-edge-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
pse-edge-pp-cli feedback --stdin < notes.txt
pse-edge-pp-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `PSE_EDGE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PSE_EDGE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
pse-edge-pp-cli profile save briefing --json
pse-edge-pp-cli --profile briefing disclosures search
pse-edge-pp-cli profile list --json
pse-edge-pp-cli profile show briefing
pse-edge-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `pse-edge-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/ngpestelos/pse-edge-pp-cli/cmd/pse-edge-pp-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add pse-edge-pp-mcp -- pse-edge-pp-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which pse-edge-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pse-edge-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pse-edge-pp-cli <command> --help`.
