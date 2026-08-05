#!/usr/bin/env bash
#
# pse-edge-pp-cli fleet installer — idempotent, macOS + Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/ngpestelos/pse-edge-pp-cli/main/scripts/install.sh | bash
#
# Prefers a prebuilt GitHub release (no local modernc.org/sqlite compile).
# Verifies the tarball against the release checksums.txt (sha256) before extract.
# Falls back to `go install` only if the download cannot be resolved.
# If the machine has the ngpestelos fleet layout (~/src/hermes-config),
# also wires the pp-pse-edge skill; skipped cleanly elsewhere.
set -euo pipefail

MODULE="github.com/ngpestelos/pse-edge-pp-cli"
BIN="pse-edge-pp-cli"
GOBIN_DIR="${GOBIN:-$HOME/.local/bin}"
OWNER_REPO="ngpestelos/pse-edge-pp-cli"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

mkdir -p "$GOBIN_DIR"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) arch="" ;;
esac

# sha256_file FILE → hex digest (macOS shasum or Linux sha256sum).
sha256_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    die "need shasum or sha256sum to verify release tarball"
  fi
}

# verify_tarball DIR TARBALL CHECKSUMS_FILE — die if hash missing or mismatch.
verify_tarball() {
  local dir="$1" tarball="$2" sums="$3"
  local expected actual
  expected="$(awk -v f="$tarball" '$2 == f { print $1; exit }' "$sums")"
  if [ -z "$expected" ]; then
    die "checksums.txt has no entry for $tarball — refusing to install unverified binary"
  fi
  actual="$(sha256_file "$dir/$tarball")"
  if [ "$actual" != "$expected" ]; then
    die "SHA-256 mismatch for $tarball (got $actual, want $expected) — refusing to install"
  fi
  log "Verified SHA-256 $tarball"
}

install_ok=false
if [ -n "$arch" ] && command -v curl >/dev/null 2>&1; then
  ver="$(curl -fsSL "https://api.github.com/repos/${OWNER_REPO}/releases/latest" 2>/dev/null \
    | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)"
  if [ -n "$ver" ]; then
    tarball="pse-edge-pp-cli_${ver}_${os}_${arch}.tar.gz"
    base="https://github.com/${OWNER_REPO}/releases/download/v${ver}"
    url="${base}/${tarball}"
    sums_url="${base}/checksums.txt"
    log "Downloading prebuilt $BIN v$ver ($os/$arch)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    if curl -fsSL "$url" -o "$tmp/$tarball" 2>/dev/null \
      && curl -fsSL "$sums_url" -o "$tmp/checksums.txt" 2>/dev/null; then
      verify_tarball "$tmp" "$tarball" "$tmp/checksums.txt"
      if tar -xzf "$tmp/$tarball" -C "$GOBIN_DIR" 2>/dev/null; then
        chmod +x "$GOBIN_DIR/pse-edge-pp-cli" "$GOBIN_DIR/pse-edge-pp-mcp" 2>/dev/null || true
        install_ok=true
      else
        warn "prebuilt extract failed ($tarball); will try building from source"
      fi
    else
      warn "prebuilt download failed ($url); will try building from source"
    fi
    rm -rf "$tmp"
    trap - EXIT
  fi
fi

if [ "$install_ok" != true ]; then
  if ! command -v go >/dev/null 2>&1; then
    die "go not on PATH. Install Go 1.21+ (https://go.dev/dl/) or download a release tarball manually."
  fi
  log "Building from source via go install (modernc.org/sqlite may be CPU-heavy)…"
  # Retry a few times for sum.golang.org flakes.
  for attempt in 1 2 3; do
    if GOTOOLCHAIN=auto GOBIN="$GOBIN_DIR" go install "${MODULE}/cmd/${BIN}@latest" 2>/tmp/pse-edge-install.err \
      && GOTOOLCHAIN=auto GOBIN="$GOBIN_DIR" go install "${MODULE}/cmd/pse-edge-pp-mcp@latest" 2>>/tmp/pse-edge-install.err; then
      install_ok=true
      break
    fi
    if grep -q "sum.golang.org" /tmp/pse-edge-install.err 2>/dev/null && [ "$attempt" -lt 3 ]; then
      warn "sum.golang.org flake (attempt $attempt/3); retrying in 10s"
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
  *) warn "$GOBIN_DIR not on PATH — add it to use $BIN" ;;
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
