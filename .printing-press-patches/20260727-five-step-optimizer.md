# Five-step optimizer — 2026-07-27

Hand patches on top of the Printing Press print (run `20260727-155656-a8a699bf`).

## Step 1–2 (requirements / delete)

| Item | Verdict |
|------|---------|
| Full PP sports-domain learning prose in repo `SKILL.md` | **Deleted** — agents need PSE install + domain hard rules + compact learning judgment; sports examples transfer uncertainty |
| Duplicate When-to-use / long Command Reference mirroring `--help` | **Cut** — runtime `which` / `--help` is SSOT |
| Platform `learn`/`store` packages | **Kept** — generated platform; do not gut |
| Dual-source quote | **Validated** — first-party EDGE + phisix convenience |

## Step 3 (optimize)

- Repo `SKILL.md` densified → **v1.2.0** (~3941w → ~compact agent skill).
- Install paths stay public GitHub (`install.sh` / go install / fleet rebuild).

## Step 4 (accelerate)

- `quote`: edge + phisix fetches concurrent per symbol; multi-ticker quotes fan out (worker cap 6), order preserved.
- `sync` already parallelized resources (pre-existing WaitGroup).

## Step 5 (automate)

- `resolveVersion` (ldflags → module → VCS → `0.0.0-dev`) so bare installs are not stuck on forever-dev.
- MCP server uses `cli.Version()`.

## Verify

```bash
GOTOOLCHAIN=auto go test ./...
pse-edge-pp-cli quote AT GTCAP --json
pse-edge-pp-cli --version
```

## Fleet follow-up

After merge: re-vendor densified skill into hermes-config `pp-pse-edge` if fleet copy lags; bump fleet skill version. Optional tag after merge if release needed.
