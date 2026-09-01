#!/bin/sh
# Installs a release package into the user's home. It copies files only:
# ts_install remains the explicit step that writes and starts a service.

set -eu

die() {
  printf 'tailscale-socks: %s\n' "$1" >&2
  exit 1
}

install_file() {
  src=$1
  dst=$2
  mode=$3
  tmp=$(mktemp "$dst.XXXXXX") || return 1
  trap 'rm -f "$tmp"' EXIT
  cp "$src" "$tmp" || return 1
  chmod "$mode" "$tmp" || return 1
  mv -f "$tmp" "$dst" || return 1
  trap - EXIT
}

kernel=$(uname -s) || die 'cannot identify the operating system'
case $kernel in
  Darwin) os=darwin; bin=tailscale-socks ;;
  Linux) os=linux; bin=tailscale-socks ;;
  MINGW*|MSYS*|CYGWIN*) os=windows; bin=tailscale-socks.exe ;;
  *) die "no release package for $kernel" ;;
esac

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

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || die 'cannot find the package directory'
[ -x "$root/$bin" ] || die "missing $bin"
[ -f "$root/install.sh" ] || die 'missing install.sh'
[ -f "$root/.env.example" ] || die 'missing .env.example'
[ -f "$root/contrib/tailscale-socks.zsh" ] || die 'missing contrib/tailscale-socks.zsh'
[ -f "$root/contrib/platform/$os.zsh" ] || die "missing contrib/platform/$os.zsh"

mkdir -p "$TSPROXY_BIN_DIR" "$TSPROXY_SHARE_DIR/contrib/platform" "$TSPROXY_ENV_DIR" ||
  die 'cannot create the installation directories'

install_file "$root/$bin" "$TSPROXY_BIN_DIR/$bin" 0755 ||
  die "cannot install $bin"
install_file "$root/contrib/tailscale-socks.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" 0644 ||
  die 'cannot install contrib/tailscale-socks.zsh'
install_file "$root/contrib/platform/$os.zsh" \
  "$TSPROXY_SHARE_DIR/contrib/platform/$os.zsh" 0644 ||
  die "cannot install contrib/platform/$os.zsh"
if [ ! -e "$env_file" ]; then
  install_file "$root/.env.example" "$env_file" 0600 ||
    die 'cannot create the initial environment file'
fi

printf 'binary:   %s\n' "$TSPROXY_BIN_DIR/$bin"
printf 'helpers:  %s\n' "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh"
printf 'config:   %s\n' "$env_file"
printf 'source:   %s\n' "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh"

case ":$PATH:" in
  *":$TSPROXY_BIN_DIR:"*) ;;
  *)
    printf 'path:     %s is not in PATH\n' "$TSPROXY_BIN_DIR" >&2
    ;;
esac
