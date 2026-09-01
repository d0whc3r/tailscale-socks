#!/usr/bin/env bats
#
# The release-package installer. It only copies files: it must never run the
# binary, which would join a tailnet, or install a service, which belongs to
# ts_install after the user has sourced the helper.

load helpers

setup() {
  isolate
  package="$BATS_TEST_TMPDIR/package"
  export TSPROXY_BIN_DIR="$BATS_TEST_TMPDIR/bin"
  export TSPROXY_SHARE_DIR="$BATS_TEST_TMPDIR/share/tailscale-socks"
  export TSPROXY_ENV_DIR="$BATS_TEST_TMPDIR/tailscale"
}

make_package() {
  local os=$1 bin=$2
  mkdir -p "$package/contrib/platform"
  printf '#!/bin/sh\nexit 42\n' > "$package/$bin"
  chmod +x "$package/$bin"
  cp "$TS_TEST_DIR/../install.sh" "$package/install.sh"
  cp "$TS_TEST_DIR/../tailscale-socks.zsh" "$package/contrib/tailscale-socks.zsh"
  printf '# backend fixture\n' > "$package/contrib/platform/$os.zsh"
  printf '# environment fixture\n' > "$package/.env.example"
}

@test "installer copies the package without running the binary" {
  case "$(uname -s)" in
    Darwin) make_package darwin tailscale-socks ;;
    Linux) make_package linux tailscale-socks ;;
    *) skip "installer fixtures cover darwin and linux runners" ;;
  esac

  run "$package/install.sh"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks" ]
  [ -f "$TSPROXY_SHARE_DIR/contrib/tailscale-socks.zsh" ]
  [ -f "$TSPROXY_SHARE_DIR/contrib/platform/$(uname -s | tr '[:upper:]' '[:lower:]' | cut -d- -f1).zsh" ]
  [ -f "$TSPROXY_ENV_DIR/.env" ]
}

@test "installer keeps an existing environment file" {
  case "$(uname -s)" in
    Darwin) make_package darwin tailscale-socks ;;
    Linux) make_package linux tailscale-socks ;;
    *) skip "installer fixtures cover darwin and linux runners" ;;
  esac
  mkdir -p "$TSPROXY_ENV_DIR"
  printf 'TS_AUTHKEY=secret\n' > "$TSPROXY_ENV_DIR/.env"

  run "$package/install.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "$TSPROXY_ENV_DIR/.env")" = "TS_AUTHKEY=secret" ]
}

@test "installer selects the windows executable and backend from Git Bash" {
  make_package windows tailscale-socks.exe
  stub_exe uname 'printf "MINGW64_NT-10.0-22631\n"'

  run "$package/install.sh"
  [ "$status" -eq 0 ]
  [ -x "$TSPROXY_BIN_DIR/tailscale-socks.exe" ]
  [ -f "$TSPROXY_SHARE_DIR/contrib/platform/windows.zsh" ]
}

@test "installer rejects an incomplete package" {
  case "$(uname -s)" in
    Darwin) make_package darwin tailscale-socks ;;
    Linux) make_package linux tailscale-socks ;;
    *) skip "installer fixtures cover darwin and linux runners" ;;
  esac
  rm "$package/contrib/tailscale-socks.zsh"

  run "$package/install.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"missing contrib/tailscale-socks.zsh"* ]]
}
