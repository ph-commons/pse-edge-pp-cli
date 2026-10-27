#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
  printf 'usage: %s DIST VERSION SOURCE_SHA GO_VERSION\n' "$0" >&2
  exit 1
fi
dist="$1"
version="$2"
revision="$3"
go_version="$4"
case "$version" in ''|*[!A-Za-z0-9.+-]*) printf 'invalid version\n' >&2; exit 1;; esac
if [[ ! "$revision" =~ ^[0-9a-f]{40}$ ]]; then
  printf 'invalid source revision\n' >&2
  exit 1
fi
cd "$dist"

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    extension=tar.gz
    [ "$os" != windows ] || extension=zip
    archive="pse-edge-pp-cli_${version}_${os}_${arch}.${extension}"
    if [ ! -f "$archive" ]; then
      printf 'missing release archive: %s\n' "$archive" >&2
      exit 1
    fi
  done
done

[ -f checksums.txt ] || { printf 'missing checksums.txt\n' >&2; exit 1; }
sha256sum -c checksums.txt
tmp_binary="$(mktemp "${TMPDIR:-/tmp}/pse-release-bin.XXXXXX")"
trap 'rm -f "$tmp_binary"' EXIT
identity='https://github.com/ph-commons/pse-edge-pp-cli/.github/workflows/release.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    extension=tar.gz
    suffix=''
    if [ "$os" = windows ]; then extension=zip; suffix=.exe; fi
    archive="pse-edge-pp-cli_${version}_${os}_${arch}.${extension}"
    for asset in "$archive" "$archive.sbom.json"; do
      [ -f "$asset" ] && [ -f "$asset.sigstore.json" ] || { printf 'missing signed asset: %s\n' "$asset" >&2; exit 1; }
      awk -v name="$asset" '$2 == name {n++} END {exit n != 1}' checksums.txt || { printf 'missing or duplicate checksum: %s\n' "$asset" >&2; exit 1; }
      cosign verify-blob --bundle "$asset.sigstore.json" "$asset" \
        --certificate-identity-regexp "$identity" \
        --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
    done
    for binary in pse-edge-pp-cli pse-edge-pp-mcp; do
      member="$binary$suffix"
      if [ "$extension" = zip ]; then
        count="$(unzip -Z1 "$archive" | awk -v name="$member" '$0 == name {n++} END {print n+0}')"
        [ "$count" = 1 ] || { printf 'missing or duplicate binary: %s in %s\n' "$member" "$archive" >&2; exit 1; }
        unzip -p "$archive" "$member" > "$tmp_binary"
      else
        count="$(tar -tzf "$archive" | awk -v name="$member" '$0 == name {n++} END {print n+0}')"
        [ "$count" = 1 ] || { printf 'missing or duplicate binary: %s in %s\n' "$member" "$archive" >&2; exit 1; }
        tar -xOzf "$archive" "$member" > "$tmp_binary"
      fi
      metadata="$(go version -m "$tmp_binary")"
      printf '%s\n' "$metadata" | awk -v expected="go$go_version" 'NR == 1 {exit $NF != expected}' || { printf 'wrong Go version: %s in %s\n' "$member" "$archive" >&2; exit 1; }
      for setting in "vcs.revision=$revision" "vcs.modified=false" "GOOS=$os" "GOARCH=$arch"; do
        printf '%s\n' "$metadata" | awk -F '\t' -v expected="$setting" '$2 == "build" && $3 == expected {n++} END {exit n != 1}' || { printf 'missing build setting %s: %s\n' "$setting" "$archive" >&2; exit 1; }
      done
      printf '%s\n' "$metadata" | awk -F '\t' -v expected="github.com/ph-commons/pse-edge-pp-cli/internal/cli.version=$version" '
        $2 == "build" && $3 ~ /^-ldflags=/ {
          flags=$3; sub(/^-ldflags="?/, "", flags); sub(/"$/, "", flags)
          n=split(flags, part, " "); for (i=1; i<n; i++) if (part[i] == "-X" && part[i+1] == expected) found++
        }
        END {exit found != 1}' || { printf 'wrong embedded version: %s in %s\n' "$member" "$archive" >&2; exit 1; }
    done
  done
done
