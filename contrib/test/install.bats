#!/usr/bin/env bats
#
# The release installer. It unpacks an archive into the user's home and does
# nothing else: it must never run the binary, which would join a tailnet, or
# install a service, which belongs to ts_install after the user has sourced
# the helper. TSPROXY_BASE_URL points curl at a fixture directory, so the
# suite stays off the network.

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

# The payload GoReleaser puts in every archive: the executable, all three zsh
# backends and the configuration template. Built for both formats every time,
# so a test that picks the wrong archive unpacks the other one instead of
# failing on a missing file.
make_assets() {
  local stage="$BATS_TEST_TMPDIR/stage"
  mkdir -p "$assets" "$stage/contrib/platform"
  printf '#!/bin/sh\n# binary fixture\nexit 42\n' > "$stage/tailscale-socks"
  printf '#!/bin/sh\n# exe fixture\nexit 42\n' > "$stage/tailscale-socks.exe"
  cp "$TS_TEST_DIR/../tailscale-socks.zsh" "$stage/contrib/tailscale-socks.zsh"
  local os
  for os in darwin linux windows; do
    printf '# %s backend fixture\n' "$os" > "$stage/contrib/platform/$os.zsh"
  done
  printf '# environment fixture\n' > "$stage/.env.example"

  tar -czf "$assets/tailscale-socks-darwin-universal.tar.gz" -C "$stage" .
  tar -czf "$assets/tailscale-socks-linux-amd64.tar.gz" -C "$stage" .
  if command -v zip > /dev/null 2>&1; then
    (cd "$stage" && zip -qr "$assets/tailscale-socks-windows-amd64.zip" .)
  fi
}

# The runner is whatever CI happens to be, so every test says which kernel and
# machine the installer sees. `uname -m` only matters off macOS.
stub_uname() {
  stub_exe uname "case \"\${1-}\" in -m) printf '${2:-x86_64}\n' ;; *) printf '$1\n' ;; esac"
}

@test "installer unpacks the macOS archive without running the binary" {
  stub_uname Darwin

  run "$installer"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks" ]
  grep -q 'binary fixture' "$TSPROXY_BIN_DIR/tailscale-socks"
  [ -f "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" ]
  grep -q 'darwin backend' "$TSPROXY_SHARE_DIR/contrib/platform/darwin.zsh"
  [ -f "$TSPROXY_ENV_DIR/.env" ]
}

@test "installer unpacks the linux archive" {
  stub_uname Linux x86_64

  run "$installer"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks" ]
  grep -q 'linux backend' "$TSPROXY_SHARE_DIR/contrib/platform/linux.zsh"
  [ ! -e "$TSPROXY_SHARE_DIR/contrib/platform/darwin.zsh" ]
}

@test "installer unpacks the windows zip into the default bin directory" {
  command -v zip > /dev/null 2>&1 || skip "zip is needed to build the fixture"
  stub_uname MINGW64_NT-10.0-22631 x86_64
  unset TSPROXY_BIN_DIR

  run "$installer"
  [ "$status" -eq 0 ]
  [ -x "$HOME/bin/tailscale-socks.exe" ]
  grep -q 'exe fixture' "$HOME/bin/tailscale-socks.exe"
  grep -q 'windows backend' "$TSPROXY_SHARE_DIR/contrib/platform/windows.zsh"
}

@test "installer keeps an existing environment file" {
  stub_uname Darwin
  mkdir -p "$TSPROXY_ENV_DIR"
  printf 'TS_AUTHKEY=secret\n' > "$TSPROXY_ENV_DIR/.env"

  run "$installer"
  [ "$status" -eq 0 ]
  [ "$(cat "$TSPROXY_ENV_DIR/.env")" = "TS_AUTHKEY=secret" ]
}

@test "installer fails when the release has no such archive" {
  stub_uname Darwin
  rm "$assets/tailscale-socks-darwin-universal.tar.gz"

  run "$installer"
  [ "$status" -ne 0 ]
  [[ "$output" == *"cannot download tailscale-socks-darwin-universal.tar.gz"* ]]
  [ ! -e "$TSPROXY_BIN_DIR/tailscale-socks" ]
}

@test "installer refuses an architecture the release does not carry" {
  stub_uname Linux aarch64

  run "$installer"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no release binary for aarch64"* ]]
  [ ! -e "$TSPROXY_BIN_DIR/tailscale-socks" ]
}
