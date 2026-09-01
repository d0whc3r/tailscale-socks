#!/usr/bin/env bats
#
# Platform detection and the backend contract. This is the part that was
# silently wrong before: every $OSTYPE that was not darwin fell through to
# systemd, so zsh on Windows failed with `systemctl: command not found`.

load helpers

@test "OSTYPE=darwin24 selects the launchd backend" {
  run zsh_run darwin24 <<'ZSH'
print -r -- "$TS_SOCKS_OS ${TS_SOCKS_PLIST:t}"
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "darwin tailscale-socks.plist" ]
}

@test "OSTYPE=linux-gnu selects the systemd backend" {
  run zsh_run linux-gnu <<'ZSH'
print -r -- "$TS_SOCKS_OS ${TS_SOCKS_UNIT:t}"
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "linux tailscale-socks.service" ]
}

@test "OSTYPE=msys selects the Task Scheduler backend" {
  run zsh_run msys <<'ZSH'
print -r -- "$TS_SOCKS_OS ${TS_SOCKS_CMD:t}"
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "windows run.cmd" ]
}

@test "OSTYPE=cygwin selects the Task Scheduler backend" {
  run zsh_run cygwin <<'ZSH'
print -r -- "$TS_SOCKS_OS"
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "windows" ]
}

@test "an unknown OSTYPE refuses to load instead of guessing" {
  run zsh_run freebsd14 <<'ZSH'
print -r -- "the snippet must not run"
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"no service backend"* ]]
  [[ "$output" != *"the snippet must not run"* ]]
}

# Every backend has to define the same set, or a command that works on one
# platform is a "command not found" on another.
contract() {
  run zsh_run "$1" <<'ZSH'
for f in _ts_svc_check _ts_installed _ts_write_service _ts_remove_service \
         _ts_start _ts_stop _ts_pid _ts_logtail _ts_logfollow _ts_boot_hint; do
  (( $+functions[$f] )) || print -r -- "missing $f"
done
print -r -- complete
ZSH
  [ "$status" -eq 0 ]
  [ "$output" = "complete" ]
}

@test "the darwin backend defines the whole contract" { contract darwin24; }
@test "the linux backend defines the whole contract"  { contract linux-gnu; }
@test "the windows backend defines the whole contract" { contract msys; }
