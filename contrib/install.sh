#!/bin/sh
# Installs tailscale-socks on macOS and Linux from the latest release:
#
#   curl -fsSL https://raw.githubusercontent.com/d0whc3r/tailscale-socks/main/contrib/install.sh | sh
#
# Everything arrives over curl on purpose. On macOS a browser download carries
# com.apple.quarantine, and Gatekeeper kills an unsigned binary that inherits
# it — `Killed: 9`, no message, nothing in the logs. curl sets no such
# attribute, which is what makes an unsigned build usable at all.
#
# It copies files only: ts_install remains the explicit step that writes and
# starts a service, and the binary is never run, which would join a tailnet.

set -eu

# Overridden by the test suite, which points it at a directory of fixtures
# through file://. Unversioned asset names, so this URL never has to change.
TSPROXY_BASE_URL=${TSPROXY_BASE_URL:-https://github.com/d0whc3r/tailscale-socks/releases/latest/download}

die() {
  printf 'tailscale-socks: %s\n' "$1" >&2
  exit 1
}

# Downloads beside the destination and renames, so a transfer that dies
# halfway leaves no partial binary behind.
fetch() {
  url=$1
  dst=$2
  mode=$3
  tmp=$(mktemp "$dst.XXXXXX") || return 1
  trap 'rm -f "$tmp"' EXIT
  curl -fsSL "$url" -o "$tmp" || return 1
  chmod "$mode" "$tmp" || return 1
  mv -f "$tmp" "$dst" || return 1
  trap - EXIT
}

kernel=$(uname -s) || die 'cannot identify the operating system'
case $kernel in
  Darwin)
    # One universal binary for Apple silicon and Intel alike, so no
    # architecture to check here.
    os=darwin
    binary_asset=tailscale-socks-darwin-universal
    ;;
  Linux)
    os=linux
    binary_asset=tailscale-socks-linux-amd64
    arch=$(uname -m) || die 'cannot identify the architecture'
    case $arch in
      x86_64 | amd64) ;;
      *) die "no release binary for $arch: build from source, see CONTRIBUTING.md" ;;
    esac
    ;;
  MINGW* | MSYS* | CYGWIN*)
    die 'no installer for Windows here: run the setup .exe from the release page'
    ;;
  *) die "no release binary for $kernel: build from source, see CONTRIBUTING.md" ;;
esac

TSPROXY_BIN_DIR=${TSPROXY_BIN_DIR:-$HOME/.local/bin}
TSPROXY_SHARE_DIR=${TSPROXY_SHARE_DIR:-$HOME/.local/share/tailscale-socks}
TSPROXY_ENV_DIR=${TSPROXY_ENV_DIR:-$HOME/.tailscale}
env_file=$TSPROXY_ENV_DIR/.env

mkdir -p "$TSPROXY_BIN_DIR" "$TSPROXY_SHARE_DIR/contrib/platform" "$TSPROXY_ENV_DIR" ||
  die 'cannot create the installation directories'

fetch "$TSPROXY_BASE_URL/$binary_asset" "$TSPROXY_BIN_DIR/tailscale-socks" 0755 ||
  die 'cannot install tailscale-socks'
if [ "$os" = darwin ]; then
  # Nothing curl writes is quarantined, so this only matters for a binary that
  # reached the machine some other way — and there it is the difference between
  # a working install and one Gatekeeper kills on sight.
  xattr -d com.apple.quarantine "$TSPROXY_BIN_DIR/tailscale-socks" 2>/dev/null || true
fi

fetch "$TSPROXY_BASE_URL/tailscale-socks.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" 0644 ||
  die 'cannot install contrib/tailscale-socks.zsh'
fetch "$TSPROXY_BASE_URL/tailscale-socks-$os.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/platform/$os.zsh" 0644 ||
  die "cannot install contrib/platform/$os.zsh"
if [ ! -e "$env_file" ]; then
  fetch "$TSPROXY_BASE_URL/tailscale-socks.env.example" "$env_file" 0600 ||
    die 'cannot create the initial environment file'
fi

printf 'binary:   %s\n' "$TSPROXY_BIN_DIR/tailscale-socks"
printf 'helpers:  %s\n' "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh"
printf 'config:   %s\n' "$env_file"
printf 'source:   %s\n' "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh"

case ":$PATH:" in
  *":$TSPROXY_BIN_DIR:"*) ;;
  *)
    printf 'path:     %s is not in PATH\n' "$TSPROXY_BIN_DIR" >&2
    ;;
esac
