#!/usr/bin/env bats
#
# The zsh completion in contrib/tailscale-socks.zsh. The candidates come from
# a stubbed binary: nothing here runs the real one, which would join a tailnet.

load helpers

setup() { isolate; }

# COMPADD replaces compsys' builtin with one that prints what it was given, so
# a completion function can be called outside a real completion.
COMPADD='compadd() { [[ $1 == -a ]] && print -l -- ${(P)2} || print -l -- ${@:#--} }'

# --- _ts_flags --------------------------------------------------------------

@test "_ts_flags takes the long flags from the binary's help" {
  stub_help_bin
  run zsh_run darwin24 <<'ZSH'
_ts_flags run
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "--accept-routes
--help
--no-accept-routes
--socks5" ]
}

@test "_ts_flags ignores a flag that only appears in the description" {
  stub_help_bin
  run zsh_run darwin24 <<'ZSH'
_ts_flags run
ZSH
  [[ "$output" != *"--not-a-flag"* ]]
}

@test "_ts_flags is silent when there is no binary" {
  export TS_SOCKS_BIN="$BATS_TEST_TMPDIR/does-not-exist"
  run zsh_run darwin24 <<'ZSH'
print -r -- "[$(_ts_flags run)]"
ZSH
  [ "$output" = "[]" ]
}

# --- _ts_settings -----------------------------------------------------------

@test "_ts_settings turns the environment names into flag names" {
  stub_help_bin
  run zsh_run darwin24 <<'ZSH'
_ts_settings
ZSH
  [ "${lines[0]}" = "socks5" ]
  [ "${lines[1]}" = "exit-node-allow-lan" ]
  [ "${#lines[@]}" -eq 2 ]
}

# --- _tailscale_socks -------------------------------------------------------

@test "_tailscale_socks completes the commands in the first position" {
  stub_help_bin
  run zsh_run darwin24 <<ZSH
$COMPADD
words=(tailscale-socks co); CURRENT=2
_tailscale_socks
ZSH
  [ "$output" = "run
status
config
upgrade" ]
}

@test "_tailscale_socks completes the default command's flags with no command" {
  stub_help_bin
  run zsh_run darwin24 <<ZSH
$COMPADD
words=(tailscale-socks --so); CURRENT=2
_tailscale_socks
ZSH
  [[ "$output" == *"--socks5"* ]]
}

@test "_tailscale_socks completes the settings after config" {
  stub_help_bin
  run zsh_run darwin24 <<ZSH
$COMPADD
words=(tailscale-socks config ex); CURRENT=3
_tailscale_socks
ZSH
  [ "$output" = "socks5
exit-node-allow-lan" ]
}

@test "_tailscale_socks completes nothing after a command that takes no argument" {
  stub_help_bin
  run zsh_run darwin24 <<ZSH
$COMPADD
words=(tailscale-socks status x); CURRENT=3
_tailscale_socks
ZSH
  [ "$output" = "" ]
}

# --- the functions ----------------------------------------------------------

@test "_ts_proxy completes its three actions" {
  run zsh_run darwin24 <<ZSH
$COMPADD
_ts_proxy
ZSH
  [ "$output" = "on
off
show" ]
}

@test "_ts_compdefs registers every function" {
  run zsh_run darwin24 <<'ZSH'
compdef() { print -l $@ }
_ts_compdefs
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"_tailscale_socks
tailscale-socks"* ]]
  [[ "$output" == *"_ts_proxy
ts_proxy"* ]]
  [[ "$output" == *"_ts_logs
ts_logs"* ]]
  [[ "$output" == *"_nothing
ts_install"* ]]
}
