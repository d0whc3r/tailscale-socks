#!/bin/sh
# Installs tailscale-socks from the latest release:
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
# Once installed, `tailscale-socks upgrade` does the same job from Go.

set -eu

# Overridden by the test suite, which points it at a directory of fixtures
# through file://. Unversioned archive names, so this URL never has to change.
TSPROXY_BASE_URL=${TSPROXY_BASE_URL:-https://github.com/d0whc3r/tailscale-socks/releases/latest/download}

die() {
  printf 'tailscale-socks: %s\n' "$1" >&2
  exit 1
}

install_file() {
  src=$1
  dst=$2
  mode=$3
  [ -f "$src" ] || return 1
  tmp=$(mktemp "$dst.XXXXXX") || return 1
  cp "$src" "$tmp" && chmod "$mode" "$tmp" && mv -f "$tmp" "$dst" && return 0
  rm -f "$tmp"
  return 1
}

kernel=$(uname -s) || die 'cannot identify the operating system'
case $kernel in
  Darwin)
    # One universal binary for Apple silicon and Intel alike, so there is no
    # architecture to check here.
    os=darwin
    bin=tailscale-socks
    archive=tailscale-socks-darwin-universal.tar.gz
    ;;
  Linux)
    os=linux
    bin=tailscale-socks
    archive=tailscale-socks-linux-amd64.tar.gz
    ;;
  MINGW* | MSYS* | CYGWIN*)
    os=windows
    bin=tailscale-socks.exe
    archive=tailscale-socks-windows-amd64.zip
    ;;
  *) die "no release for $kernel: build from source, see CONTRIBUTING.md" ;;
esac

if [ "$os" != darwin ]; then
  arch=$(uname -m) || die 'cannot identify the architecture'
  case $arch in
    x86_64 | amd64) ;;
    *) die "no release binary for $arch: build from source, see CONTRIBUTING.md" ;;
  esac
fi

if [ -z "${TSPROXY_BIN_DIR:-}" ]; then
  if [ "$os" = windows ]; then
    TSPROXY_BIN_DIR=$HOME/bin
  else
    TSPROXY_BIN_DIR=$HOME/.local/bin
  fi
fi
TSPROXY_SHARE_DIR=${TSPROXY_SHARE_DIR:-$HOME/.local/share/tailscale-socks}
TSPROXY_ENV_DIR=${TSPROXY_ENV_DIR:-$HOME/.tailscale}
env_file=$TSPROXY_ENV_DIR/.env

stage=$(mktemp -d) || die 'cannot create a temporary directory'
trap 'rm -rf "$stage"' EXIT

curl -fsSL "$TSPROXY_BASE_URL/$archive" -o "$stage/$archive" ||
  die "cannot download $archive"

case $archive in
  *.zip)
    # bsdtar reads a zip and ships with Windows as tar.exe; GNU tar does not,
    # which is why unzip comes first.
    if command -v unzip >/dev/null 2>&1; then
      unzip -q "$stage/$archive" -d "$stage" || die "cannot unpack $archive"
    else
      tar -xf "$stage/$archive" -C "$stage" || die "cannot unpack $archive: install unzip"
    fi
    ;;
  *) tar -xzf "$stage/$archive" -C "$stage" || die "cannot unpack $archive" ;;
esac

mkdir -p "$TSPROXY_BIN_DIR" "$TSPROXY_SHARE_DIR/contrib/platform" "$TSPROXY_ENV_DIR" ||
  die 'cannot create the installation directories'

install_file "$stage/$bin" "$TSPROXY_BIN_DIR/$bin" 0755 || die "cannot install $bin"
if [ "$os" = darwin ]; then
  # Nothing curl writes is quarantined, so this only matters for a binary that
  # reached the machine some other way — and there it is the difference between
  # a working install and one Gatekeeper kills on sight.
  xattr -d com.apple.quarantine "$TSPROXY_BIN_DIR/$bin" 2>/dev/null || true
fi

install_file "$stage/contrib/tailscale-socks.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" 0644 ||
  die 'cannot install contrib/tailscale-socks.zsh'
install_file "$stage/contrib/platform/$os.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/platform/$os.zsh" 0644 ||
  die "cannot install contrib/platform/$os.zsh"
if [ ! -e "$env_file" ]; then
  install_file "$stage/.env.example" "$env_file" 0600 ||
    die 'cannot create the initial environment file'
fi

# The helpers are zsh, so ~/.zshrc is the only file worth touching. ZDOTDIR
# moves it, when the user has set one.
zshrc=${ZDOTDIR:-$HOME}/.zshrc
helper=$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh
# $HOME left unexpanded when the helper lives under it, so the line survives a
# home directory that moves or gets renamed.
case $helper in
  "$HOME"/*) source_line="source \"\$HOME${helper#"$HOME"}\"" ;;
  *) source_line="source \"$helper\"" ;;
esac

# Matched on the path alone, not on the whole line: a hand-edited or
# deliberately commented-out source counts as present and is left as it is.
if [ -e "$zshrc" ] && grep -qF 'contrib/tailscale-socks.zsh' "$zshrc"; then
  zshrc_state='already sources the helpers'
else
  printf '\n# tailscale-socks\n%s\n' "$source_line" >> "$zshrc" ||
    die "cannot write $zshrc"
  zshrc_state='source line added'
fi

printf 'binary:   %s\n' "$TSPROXY_BIN_DIR/$bin"
printf 'helpers:  %s\n' "$helper"
printf 'config:   %s\n' "$env_file"
printf 'zshrc:    %s (%s)\n' "$zshrc" "$zshrc_state"

case ":$PATH:" in
  *":$TSPROXY_BIN_DIR:"*) ;;
  *)
    printf 'path:     %s is not in PATH\n' "$TSPROXY_BIN_DIR" >&2
    ;;
esac
