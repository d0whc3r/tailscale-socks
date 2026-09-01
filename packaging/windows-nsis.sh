#!/bin/sh
# Builds the Windows installer for one architecture. Called from the GoReleaser
# build post-hook, which fires once per target:
#
#   windows-nsis.sh OS ARCH BINARY VERSION
#
# Every target that is not Windows is a no-op — the hook has no condition of
# its own, so the guard lives here.

set -eu

os=${1:?os}
arch=${2:?arch}
bin=${3:?binary}
version=${4:?version}

[ "$os" = windows ] || exit 0

command -v makensis >/dev/null 2>&1 ||
  { printf 'packaging: makensis not found: brew install makensis (apt: nsis)\n' >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

mkdir -p "$stage/contrib/platform"
cp "$bin" "$stage/tailscale-socks.exe"
cp "$root/contrib/tailscale-socks.zsh" "$stage/contrib/tailscale-socks.zsh"
cp "$root/contrib/platform/windows.zsh" "$stage/contrib/platform/windows.zsh"
cp "$root/packaging/path.ps1" "$stage/path.ps1"
# Named .env, and the install directory holds the executable, so the binary
# reads this one directly — every line in it is commented, so an untouched
# copy changes nothing and ~/.tailscale/.env still applies.
cp "$root/.env.example" "$stage/.env"
cp "$root/README.md" "$stage/README.md"
mkdir -p "$stage/docs"
cp "$root"/docs/*.md "$stage/docs/"
cp "$root/LICENSE" "$stage/LICENSE"

# VIProductVersion takes four numbers and nothing else; a snapshot version
# carries a commit suffix and a release one has three components.
viversion=$(printf '%s' "$version" | sed 's/[^0-9.].*$//; s/\.$//')
case $viversion in
  '') viversion=0.0.0.0 ;;
  *.*.*.*) ;;
  *.*.*) viversion=$viversion.0 ;;
  *.*) viversion=$viversion.0.0 ;;
  *) viversion=$viversion.0.0.0 ;;
esac

out=$root/dist/tailscale-socks-setup-windows-$arch.exe
mkdir -p "$root/dist"
makensis -V2 \
  "-DSTAGE=$stage" \
  "-DVERSION=$version" \
  "-DVIVERSION=$viversion" \
  "-DARCH=$arch" \
  "-DOUTFILE=$out" \
  "$root/packaging/windows.nsi"

printf 'packaging: %s\n' "$out"
