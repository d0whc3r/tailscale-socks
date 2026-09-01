#!/usr/bin/env bats
#
# ts_status and the lifecycle commands. ts_status deliberately never runs
# `tailscale-socks status`: that would join the tailnet with the same state
# directory and node key as the running service.

load helpers

setup() { isolate; stub_bin; }

@test "ts_status reports a service that was never installed" {
  run zsh_run darwin24 <<'ZSH'
ts_status
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"service:"*"not installed"* ]]
}

@test "ts_status reports an installed but stopped service" {
  run zsh_run darwin24 <<'ZSH'
_ts_installed() { return 0 }
_ts_pid() { : }
ts_status
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"service:"*"stopped"* ]]
}

@test "ts_status probes every enabled listener and names the disabled ones" {
  export TS_STUB_socks5=127.0.0.1:45999 TS_STUB_http=127.0.0.1:45998
  run zsh_run darwin24 <<'ZSH'
_ts_installed() { return 0 }
_ts_pid() { print -r -- 4242 }
_ts_logtail() { : }
ts_status
ZSH
  [[ "${lines[0]}" == "service:  running (pid 4242)" ]]
  [[ "${lines[1]}" == "socks5:   127.0.0.1:45999 NOT LISTENING" ]]
  [[ "${lines[2]}" == "http:     127.0.0.1:45998 NOT LISTENING" ]]
  [[ "${lines[3]}" == "dns:      disabled" ]]
}

@test "ts_status marks a listener that answers as up" {
  run zsh_run darwin24 <<'ZSH'
zmodload zsh/net/tcp
for port in {45400..45460}; do ztcp -l $port 2>/dev/null && { lfd=$REPLY; break }; done
_ts_installed() { return 0 }
_ts_pid() { print -r -- 4242 }
_ts_cfg() { [[ $1 == socks5 ]] && print -r -- 127.0.0.1:$port }
_ts_logtail() { : }
ts_status
ztcp -c $lfd
ZSH
  [[ "${lines[1]}" == "socks5:   127.0.0.1:"*" up" ]]
}

@test "ts_status appends the node summary the running process printed" {
  run zsh_run darwin24 <<'ZSH'
_ts_installed() { return 0 }
_ts_pid() { print -r -- 4242 }
_ts_cfg() { : }
_ts_logtail() { print -l "node: my-proxy.tailnet.ts.net (Running)" }
ts_status
ZSH
  [[ "$output" == *"node: my-proxy.tailnet.ts.net (Running)"* ]]
}

@test "ts_status never runs the binary's own status subcommand" {
  export TS_STUB_socks5=127.0.0.1:45999
  run zsh_run darwin24 <<'ZSH'
_ts_installed() { return 0 }
_ts_pid() { print -r -- 4242 }
_ts_logtail() { : }
# A second node on the same state directory is the failure this guards.
tailscale-socks() { [[ $1 == status ]] && print -r -- "JOINED THE TAILNET" }
ts_status
ZSH
  [[ "$output" != *"JOINED THE TAILNET"* ]]
}

# --- lifecycle --------------------------------------------------------------

@test "ts_up refuses to start a service that is not installed" {
  run zsh_run darwin24 <<'ZSH'
ts_up
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"not installed; run ts_install"* ]]
}

@test "ts_down refuses when nothing is installed" {
  run zsh_run darwin24 <<'ZSH'
ts_down
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"not installed"* ]]
}

@test "ts_uninstall on a clean machine says so and succeeds" {
  run zsh_run darwin24 <<'ZSH'
ts_uninstall
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "not installed" ]
}

@test "ts_install checks the service manager before touching the binary" {
  run zsh_run darwin24 <<'ZSH'
_ts_svc_check() { print -u2 "no service manager"; return 1 }
_ts_bin() { print -u2 "_ts_bin must not run"; return 1 }
ts_install
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"no service manager"* ]]
  [[ "$output" != *"_ts_bin must not run"* ]]
}

@test "ts_install stops when the service cannot be written" {
  run zsh_run darwin24 <<'ZSH'
_ts_svc_check() { return 0 }
_ts_write_service() { return 1 }
ts_up() { print -r -- "ts_up must not run" }
ts_install
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" != *"ts_up must not run"* ]]
}

@test "ts_install starts the service and names the binary it wired in" {
  stub_exe launchctl ':'
  run zsh_run darwin24 <<'ZSH'
launchctl() { : }
ts_install
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"installed: "*"/tailscale-socks"* ]]
  [[ "$output" == *"autostart:"* ]]
  [[ "$output" == *"started"* ]]
}

@test "ts_restart does not start when the stop failed" {
  run zsh_run darwin24 <<'ZSH'
_ts_installed() { return 0 }
_ts_stop() { return 1 }
_ts_start() { print -r -- "started anyway" }
ts_restart
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" != *"started anyway"* ]]
}

@test "ts_logs -f follows and a bare ts_logs tails 50 lines" {
  run zsh_run darwin24 <<'ZSH'
_ts_logfollow() { print -r -- "follow" }
_ts_logtail() { print -r -- "tail $1" }
ts_logs -f
ts_logs
ts_logs 200
ZSH
  [ "${lines[0]}" = "follow" ]
  [ "${lines[1]}" = "tail 50" ]
  [ "${lines[2]}" = "tail 200" ]
}
