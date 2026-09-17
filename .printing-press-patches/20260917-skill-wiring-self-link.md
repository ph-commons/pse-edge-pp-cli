# Skill wiring self-link (fleet layout)

Hand patch on top of the Printing Press print. Do not drop on reprint.

## Problem

The printed `scripts/install.sh` ends with a "Fleet skill wiring (optional)"
block that unconditionally ran:

```sh
ln -sfn "$SKILL_SRC" "$HOME/.claude/skills/pp-pse-edge"
```

In the fleet layout `~/.claude/skills` is itself a dir-level symlink at the
canonical skills tree (`~/src/hermes-config/skills`) — that is the intended
cross-agent install, one directory-level link per agent rather than a per-skill
farm. `$HOME/.claude/skills/pp-pse-edge` therefore *is* the canonical skill
directory, and the command wrote a link inside it:

```
hermes-config/skills/pp-pse-edge/pp-pse-edge -> /Users/<user>/src/hermes-config/skills/pp-pse-edge
```

That link is redundant, and being git-tracked with an absolute machine-specific
target it made the file read as modified on every other machine, so each
machine's auto-sync committed the flip. Observed 2026-09-14: a
`git pull --rebase` conflict on exactly that path, and a permanently dirty tree.

## Fix

Gate the per-skill link on reachability instead of on the skills dir being a
real directory: skip only when `$HOME/.claude/skills/pp-pse-edge` already
resolves to `$SKILL_SRC`.

```sh
if [ -d "$SKILL_SRC" ] && [ -d "$HOME/.claude/skills" ] && [ "$(cd "$HOME/.claude/skills/pp-pse-edge" 2>/dev/null && pwd -P)" != "$(cd "$SKILL_SRC" 2>/dev/null && pwd -P)" ]; then
```

`pwd -P` is POSIX, so no `readlink -f` dependency. Behaviour:

| `~/.claude/skills` | outcome |
|---|---|
| symlink at the canonical tree (fleet) | skipped; the canonical dir stays untouched |
| real directory | wired; re-runs are no-ops once the link resolves |
| symlink elsewhere | wired into that dir |
| no `~/src/hermes-config` | skipped |

## Files

- `scripts/install.sh`
- `CHANGELOG.md`

## Verify

```bash
bash -n scripts/install.sh
go test ./...
```

Behavioural check (extract the shipped block and source it in a scratch `HOME`,
so it tests the real text rather than a copy):

```bash
SB="$(mktemp -d)"
mkdir -p "$SB/src/hermes-config/skills/pp-pse-edge" \
         "$SB/case-a/.claude" "$SB/case-b/.claude/skills" \
         "$SB/case-c/.claude" "$SB/case-c/other-skills"
ln -s "$SB/src/hermes-config/skills" "$SB/case-a/.claude/skills"
ln -s "$SB/case-c/other-skills" "$SB/case-c/.claude/skills"
sed -n '/^# Fleet skill wiring/,/^fi$/p' scripts/install.sh > "$SB/block.sh"
run() { HOME="$1" HERMES_CONFIG="$SB/src/hermes-config" \
        bash -c 'log(){ :; }; source "$1"' _ "$SB/block.sh"; }
run "$SB/case-a"                                    # fleet
! test -e "$SB/src/hermes-config/skills/pp-pse-edge/pp-pse-edge"
run "$SB/case-b"; run "$SB/case-b"                  # real dir, idempotent
[ "$(readlink "$SB/case-b/.claude/skills/pp-pse-edge")" = "$SB/src/hermes-config/skills/pp-pse-edge" ]
run "$SB/case-c"                                    # symlink elsewhere
[ "$(readlink "$SB/case-c/.claude/skills/pp-pse-edge")" = "$SB/src/hermes-config/skills/pp-pse-edge" ]
```

The check is discriminating: case A fails against the unpatched block, which
creates the self-referential link.

## Changelog

Entry under `CHANGELOG.md` → `## [Unreleased]` / Fixed (policy: hand-maintained
for this independent repo as of 2026-08-04).
