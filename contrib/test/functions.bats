#!/usr/bin/env bats
#
# The shared helpers in contrib/tailscale-socks.zsh. All of them are pure or
# reach no further than a local socket: nothing here joins a tailnet.

load helpers

setup() { isolate; }

# --- _ts_bin ----------------------------------------------------------------

@test "_ts_bin resolves the binary from \$PATH" {
  stub_bin
  run zsh_run darwin24 <<'ZSH'
print -r -- ${$(_ts_bin):t}
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "tailscale-socks" ]
}

@test "_ts_bin prefers TS_SOCKS_BIN over \$PATH" {
  stub_bin
  mkdir -p "$BATS_TEST_TMPDIR/other"
  cp "$BATS_TEST_TMPDIR/bin/tailscale-socks" "$BATS_TEST_TMPDIR/other/ts-override"
  export TS_SOCKS_BIN="$BATS_TEST_TMPDIR/other/ts-override"
  run zsh_run darwin24 <<'ZSH'
print -r -- ${$(_ts_bin):t}
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "ts-override" ]
}

@test "_ts_bin resolves a symlink, so the service does not depend on \$PATH" {
  stub_bin
  ln -s "$BATS_TEST_TMPDIR/bin/tailscale-socks" "$BATS_TEST_TMPDIR/link"
  export TS_SOCKS_BIN="$BATS_TEST_TMPDIR/link"
  run zsh_run darwin24 <<'ZSH'
print -r -- ${$(_ts_bin):t}
ZSH
  [ "$output" = "tailscale-socks" ]
}

@test "_ts_bin fails with an actionable message when there is no binary" {
  export TS_SOCKS_BIN="$BATS_TEST_TMPDIR/does-not-exist"
  run zsh_run darwin24 <<'ZSH'
_ts_bin
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"set TS_SOCKS_BIN="* ]]
}

# --- _ts_cfg ----------------------------------------------------------------

@test "_ts_cfg returns one setting and drops the binary's stderr" {
  stub_bin
  export TS_STUB_socks5=127.0.0.1:1080
  run zsh_run darwin24 <<'ZSH'
print -r -- "[$(_ts_cfg socks5)]"
ZSH
  [ "$output" = "[127.0.0.1:1080]" ]
}

@test "_ts_cfg returns empty for a disabled listener" {
  stub_bin
  run zsh_run darwin24 <<'ZSH'
print -r -- "[$(_ts_cfg dns)]"
ZSH
  [ "$output" = "[]" ]
}

# --- _ts_probe --------------------------------------------------------------
#
# The listener is opened in the same zsh that probes it. A connect succeeds
# off the backlog without an accept, so no second process is needed and the
# test never leaves the loopback interface.

@test "_ts_probe succeeds against a listening port" {
  run zsh_run darwin24 <<'ZSH'
zmodload zsh/net/tcp
for port in {45000..45060}; do ztcp -l $port 2>/dev/null && { lfd=$REPLY; break }; done
[[ -z $lfd ]] && { print -u2 "no free port"; exit 1 }
_ts_probe 127.0.0.1:$port && print -r -- up || print -r -- down
ztcp -c $lfd
ZSH
  [ "$output" = "up" ]
}

@test "_ts_probe fails against a closed port" {
  run zsh_run darwin24 <<'ZSH'
zmodload zsh/net/tcp
for port in {45100..45160}; do ztcp -l $port 2>/dev/null && { ztcp -c $REPLY; break }; done
_ts_probe 127.0.0.1:$port && print -r -- up || print -r -- down
ZSH
  [ "$output" = "down" ]
}

@test "_ts_probe defaults a bare :port to 127.0.0.1" {
  run zsh_run darwin24 <<'ZSH'
zmodload zsh/net/tcp
for port in {45200..45260}; do ztcp -l $port 2>/dev/null && { lfd=$REPLY; break }; done
_ts_probe :$port && print -r -- up || print -r -- down
ztcp -c $lfd
ZSH
  [ "$output" = "up" ]
}

@test "_ts_probe strips the brackets from an IPv6 address" {
  run zsh_run darwin24 <<'ZSH'
zmodload zsh/net/tcp
for port in {45300..45360}; do ztcp -l $port 2>/dev/null && { lfd=$REPLY; break }; done
_ts_probe "[127.0.0.1]:$port" && print -r -- up || print -r -- down
ztcp -c $lfd
ZSH
  [ "$output" = "up" ]
}

# --- _ts_summary ------------------------------------------------------------

@test "_ts_summary takes the node block and stops at the next log line" {
  run zsh_run darwin24 <<'ZSH'
_ts_logtail() {
  print -l \
    "2026/09/01 10:00:00 config: loaded /home/x/.tailscale/.env" \
    "node: my-proxy.tailnet.ts.net (Running)" \
    "exit node: gateway.tailnet.ts.net online=true" \
    "2026/09/01 10:00:05 socks5 listening on 127.0.0.1:1080"
}
_ts_summary
ZSH
  [ "${lines[0]}" = "node: my-proxy.tailnet.ts.net (Running)" ]
  [ "${lines[1]}" = "exit node: gateway.tailnet.ts.net online=true" ]
  [ "${#lines[@]}" -eq 2 ]
}

@test "_ts_summary is empty when the log has no node block" {
  run zsh_run darwin24 <<'ZSH'
_ts_logtail() { print -l "2026/09/01 10:00:00 starting" }
print -r -- "[$(_ts_summary)]"
ZSH
  [ "$output" = "[]" ]
}

@test "_ts_summary keeps the last node block when the service restarted" {
  run zsh_run darwin24 <<'ZSH'
_ts_logtail() {
  print -l \
    "node: old.tailnet.ts.net (Running)" \
    "2026/09/01 10:00:05 restarting" \
    "node: new.tailnet.ts.net (Running)"
}
_ts_summary
ZSH
  [ "$output" = "node: new.tailnet.ts.net (Running)" ]
}
