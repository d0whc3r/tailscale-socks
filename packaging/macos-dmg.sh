#!/bin/sh
# Builds the macOS disk image from the universal binary. Called from the
# GoReleaser universal_binaries post-hook:
#
#   macos-dmg.sh BINARY VERSION OUTFILE
#
# xorriso, not hdiutil: hdiutil is macOS-only and would pin the whole release
# job to a macOS runner for this one artifact. macOS mounts an ISO 9660 image
# with Rock Ridge extensions, which is what a .dmg has to be to carry Unix
# permissions — the executable bit on the binary and on install.command.
#
# The image is not compressed. UDIF, the compressed .dmg format, needs
# hdiutil; the payload is a Go binary that is already stripped, so the whole
# gain would be the zlib pass hdiutil does on top.

set -eu

bin=${1:?binary}
version=${2:?version}
out=${3:?output}

command -v xorriso >/dev/null 2>&1 ||
  { printf 'packaging: xorriso not found: brew install xorriso (apt: xorriso)\n' >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# Same tree as the release archive, so contrib/install.sh finds what it
# checks for, plus the wrapper that makes it double-clickable. That is what
# the image is for: an installer a Mac user can run without a terminal.
mkdir -p "$stage/contrib/platform"
install -m 0755 "$bin" "$stage/tailscale-socks"
install -m 0755 "$root/contrib/install.sh" "$stage/install.sh"
install -m 0755 "$root/packaging/install.command" "$stage/install.command"
install -m 0644 "$root/contrib/tailscale-socks.zsh" "$stage/contrib/tailscale-socks.zsh"
install -m 0644 "$root/contrib/platform/darwin.zsh" "$stage/contrib/platform/darwin.zsh"
install -m 0644 "$root/.env.example" "$stage/.env.example"
install -m 0644 "$root/README.md" "$stage/README.md"
install -d -m 0755 "$stage/docs"
install -m 0644 "$root"/docs/*.md "$stage/docs/"
install -m 0644 "$root/LICENSE" "$stage/LICENSE"

mkdir -p "$(dirname -- "$out")"
xorriso -as mkisofs \
  -quiet \
  -r \
  -V tailscale-socks \
  -o "$out" \
  "$stage"

printf 'packaging: %s (%s)\n' "$out" "$version"
