#!/usr/bin/env bats
#
# The macOS and Linux installer. It fetches the release into the user's home
# and does nothing else: it must never run the binary, which would join a
# tailnet, or install a service, which belongs to ts_install after the user
# has sourced the helper. TSPROXY_BASE_URL points curl at a fixture directory,
# so the suite stays off the network.

load helpers

setup() {
  isolate
  assets="$BATS_TEST_TMPDIR/assets"
  export TSPROXY_BASE_URL="file://$assets"
  export TSPROXY_BIN_DIR="$BATS_TEST_TMPDIR/bin"
  export TSPROXY_SHARE_DIR="$BATS_TEST_TMPDIR/share/tailscale-socks"
  export TSPROXY_ENV_DIR="$BATS_TEST_TMPDIR/tailscale"
  installer="$TS_TEST_DIR/../install.sh"
  make_assets
}

# Both platforms every time, so a test that picks the wrong one installs the
# other fixture instead of failing on a missing file.
make_assets() {
  mkdir -p "$assets"
  printf '#!/bin/sh\n# darwin fixture\nexit 42\n' > "$assets/tailscale-socks-darwin-universal"
  printf '#!/bin/sh\n# linux fixture\nexit 42\n' > "$assets/tailscale-socks-linux-amd64"
  cp "$TS_TEST_DIR/../tailscale-socks.zsh" "$assets/tailscale-socks.zsh"
  printf '# darwin backend fixture\n' > "$assets/tailscale-socks-darwin.zsh"
  printf '# linux backend fixture\n' > "$assets/tailscale-socks-linux.zsh"
  printf '# environment fixture\n' > "$assets/tailscale-socks.env.example"
}

# The runner is whatever CI happens to be, so every test says which kernel and
# machine the installer sees. `uname -m` only matters on Linux.
stub_uname() {
  stub_exe uname "case \"\${1-}\" in -m) printf '${2:-x86_64}\n' ;; *) printf '$1\n' ;; esac"
}

@test "installer fetches the macOS release without running the binary" {
  stub_uname Darwin

  run "$installer"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks" ]
  grep -q 'darwin fixture' "$TSPROXY_BIN_DIR/tailscale-socks"
  [ -f "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" ]
  [ -f "$TSPROXY_SHARE_DIR/contrib/platform/darwin.zsh" ]
  [ -f "$TSPROXY_ENV_DIR/.env" ]
}

@test "installer fetches the linux release without running the binary" {
  stub_uname Linux x86_64

  run "$installer"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks" ]
  grep -q 'linux fixture' "$TSPROXY_BIN_DIR/tailscale-socks"
  [ -f "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" ]
  [ -f "$TSPROXY_SHARE_DIR/contrib/platform/linux.zsh" ]
  [ -f "$TSPROXY_ENV_DIR/.env" ]
}

@test "installer keeps an existing environment file" {
  stub_uname Darwin
  mkdir -p "$TSPROXY_ENV_DIR"
  printf 'TS_AUTHKEY=secret\n' > "$TSPROXY_ENV_DIR/.env"

  run "$installer"
  [ "$status" -eq 0 ]
  [ "$(cat "$TSPROXY_ENV_DIR/.env")" = "TS_AUTHKEY=secret" ]
}

@test "installer fails on a missing asset" {
  stub_uname Darwin
  rm "$assets/tailscale-socks-darwin.zsh"

  run "$installer"
  [ "$status" -ne 0 ]
  [[ "$output" == *"cannot install contrib/platform/darwin.zsh"* ]]
}

@test "installer sends Windows to the setup executable" {
  stub_uname MINGW64_NT-10.0-22631

  run "$installer"
  [ "$status" -ne 0 ]
  [[ "$output" == *"run the setup .exe"* ]]
  [ ! -e "$TSPROXY_BIN_DIR/tailscale-socks" ]
}

@test "installer refuses an architecture the release does not carry" {
  stub_uname Linux aarch64

  run "$installer"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no release binary for aarch64"* ]]
  [ ! -e "$TSPROXY_BIN_DIR/tailscale-socks" ]
}
