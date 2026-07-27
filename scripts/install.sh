#!/usr/bin/env bash
#
# pse-edge-pp-cli fleet installer — idempotent, macOS + Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/ngpestelos/pse-edge-pp-cli/main/scripts/install.sh | bash
#
# Prefers the prebuilt GitHub release (no local modernc.org/sqlite compile).
# Falls back to `go install` only when the download cannot be resolved.
# If this machine has the ngpestelos fleet layout (~/src/hermes-config),
# also wires the pp-pse-edge skill; skipped cleanly elsewhere.

set -euo pipefail

MODULE="github.com/ngpestelos/pse-edge-pp-cli"
BIN="pse-edge-pp-cli"
GOBIN_DIR="${GOBIN:-$HOME/.local/bin}"
OWNER_REPO="ngpestelos/pse-edge-pp-cli"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

mkdir -p "$GOBIN_DIR"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) arch="" ;;
esac

install_ok=false
if [ -n "$arch" ] && command -v curl >/dev/null 2>&1; then
  ver="$(curl -fsSL "https://api.github.com/repos/${OWNER_REPO}/releases/latest" 2>/dev/null \
    | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)"
  if [ -n "$ver" ]; then
    tarball="pse-edge-pp-cli_${ver}_${os}_${arch}.tar.gz"
    url="https://github.com/${OWNER_REPO}/releases/download/v${ver}/${tarball}"
    log "Downloading prebuilt $BIN v$ver ($os/$arch)"
    tmp="$(mktemp -d)"
    if curl -fsSL "$url" -o "$tmp/$tarball" 2>/dev/null && tar -xzf "$tmp/$tarball" -C "$GOBIN_DIR" 2>/dev/null; then
      chmod +x "$GOBIN_DIR/pse-edge-pp-cli" "$GOBIN_DIR/pse-edge-pp-mcp" 2>/dev/null || true
      install_ok=true
    else
      warn "prebuilt download failed ($url); will try building from source."
    fi
    rm -rf "$tmp"
  fi
fi

if [ "$install_ok" != true ]; then
  command -v go >/dev/null 2>&1 || die "No prebuilt binary available and Go not on PATH. Install Go 1.21+ (https://go.dev/dl/) or check release assets."
  warn "Building from source — this compiles modernc.org/sqlite and is CPU-heavy."
  for attempt in 1 2 3; do
    if GOTOOLCHAIN=auto GOBIN="$GOBIN_DIR" go install "${MODULE}/cmd/${BIN}@latest" 2>/tmp/pse-edge-install.err; then
      install_ok=true
      break
    fi
    if grep -q "sum.golang.org" /tmp/pse-edge-install.err 2>/dev/null; then
      warn "checksum DB not ready (attempt $attempt/3); retrying in 10s"
      sleep 10
    else
      cat /tmp/pse-edge-install.err >&2
      break
    fi
  done
  rm -f /tmp/pse-edge-install.err
fi
[ "$install_ok" = true ] || die "install failed (neither prebuilt download nor go install worked)."

log "Installed: $($GOBIN_DIR/$BIN --version 2>/dev/null || echo "$GOBIN_DIR/$BIN")"
case ":$PATH:" in
  *":$GOBIN_DIR:"*) ;;
  *) warn "$GOBIN_DIR is not on PATH — add it to use $BIN" ;;
esac

# Fleet skill wiring (optional)
SKILL_SRC="${HERMES_CONFIG:-$HOME/src/hermes-config}/skills/pp-pse-edge"
if [ -d "$SKILL_SRC" ] && [ -d "$HOME/.claude/skills" ]; then
  ln -sfn "$SKILL_SRC" "$HOME/.claude/skills/pp-pse-edge"
  log "Linked ~/.claude/skills/pp-pse-edge"
fi

log "Smoke: $BIN session --json"
"$GOBIN_DIR/$BIN" session --json >/dev/null || warn "session smoke failed (network?)"
log "Done."
