#!/usr/bin/env bats
#
# ts_proxy. The invariant worth guarding is socks5h, not socks5: the proxy has
# to resolve the name on the tailnet side, or MagicDNS names do not exist for
# the client.

load helpers

setup() { isolate; stub_bin; }

@test "ts_proxy on exports the http and socks5 variables" {
  export TS_STUB_socks5=127.0.0.1:1080 TS_STUB_http=127.0.0.1:8080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
print -l -- "$http_proxy" "$https_proxy" "$HTTP_PROXY" "$HTTPS_PROXY" "$ALL_PROXY" "$all_proxy"
ZSH
  [ "${lines[0]}" = "http://127.0.0.1:8080" ]
  [ "${lines[1]}" = "http://127.0.0.1:8080" ]
  [ "${lines[2]}" = "http://127.0.0.1:8080" ]
  [ "${lines[3]}" = "http://127.0.0.1:8080" ]
  [ "${lines[4]}" = "socks5h://127.0.0.1:1080" ]
  [ "${lines[5]}" = "socks5h://127.0.0.1:1080" ]
}

@test "ts_proxy on uses socks5h, so names resolve on the tailnet" {
  export TS_STUB_socks5=127.0.0.1:1080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
print -r -- "$ALL_PROXY"
ZSH
  [ "$output" = "socks5h://127.0.0.1:1080" ]
  [[ "$output" != "socks5://"* ]]
}

@test "ts_proxy on sets the local bypass list" {
  export TS_STUB_socks5=127.0.0.1:1080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
print -l -- "$NO_PROXY" "$no_proxy"
ZSH
  [ "${lines[0]}" = "localhost,127.0.0.1,::1" ]
  [ "${lines[1]}" = "localhost,127.0.0.1,::1" ]
}

@test "ts_proxy on sets no variable for a disabled listener" {
  export TS_STUB_socks5=127.0.0.1:1080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
print -r -- "http=[$http_proxy] socks=[$ALL_PROXY]"
ZSH
  [ "$output" = "http=[] socks=[socks5h://127.0.0.1:1080]" ]
}

@test "ts_proxy on drops a variable left by an earlier call" {
  export TS_STUB_socks5=127.0.0.1:1080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
export http_proxy=http://stale:9999 HTTPS_PROXY=http://stale:9999
ts_proxy on >/dev/null 2>&1
print -r -- "http=[$http_proxy] https=[$HTTPS_PROXY]"
ZSH
  [ "$output" = "http=[] https=[]" ]
}

@test "ts_proxy on fails when no listener is enabled" {
  run zsh_run darwin24 <<'ZSH'
ts_proxy on
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"no proxy listener is enabled"* ]]
}

@test "ts_proxy on warns when nothing is listening on an enabled address" {
  export TS_STUB_socks5=127.0.0.1:45999
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null
ZSH
  [[ "$stderr" == *"nothing is listening on 127.0.0.1:45999"* ]]
}

@test "ts_proxy off unsets every variable it set" {
  export TS_STUB_socks5=127.0.0.1:1080 TS_STUB_http=127.0.0.1:8080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
ts_proxy off >/dev/null
print -r -- "[$http_proxy$https_proxy$HTTP_PROXY$HTTPS_PROXY$ALL_PROXY$all_proxy$NO_PROXY$no_proxy]"
ZSH
  [ "$output" = "[]" ]
}

@test "ts_proxy show reports off, and fails, when nothing is set" {
  run zsh_run darwin24 <<'ZSH'
ts_proxy show
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"proxy:"*"off"* ]]
}

@test "ts_proxy with no argument shows" {
  export TS_STUB_socks5=127.0.0.1:1080
  run --separate-stderr zsh_run darwin24 <<'ZSH'
ts_proxy on >/dev/null 2>&1
ts_proxy
ZSH
  [[ "$output" == *"socks5:"*"socks5h://127.0.0.1:1080"* ]]
}

@test "ts_proxy rejects an unknown argument with usage" {
  run zsh_run darwin24 <<'ZSH'
ts_proxy sideways
ZSH
  [ "$status" -eq 2 ]
  [[ "$output" == *"usage: ts_proxy [on|off|show]"* ]]
}
