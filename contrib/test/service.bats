#!/usr/bin/env bats
#
# The three service backends. Each one is exercised on whatever machine runs
# the suite: $OSTYPE is chosen by the harness and the service manager is a
# shell function, so the Windows task is verified from macOS and the launchd
# agent from Linux. Nothing is installed and no node is started.

load helpers

setup() { isolate; stub_bin; }

# --- macOS: launchd ---------------------------------------------------------

@test "darwin writes a plist that runs the resolved binary" {
  run zsh_run darwin24 <<'ZSH'
launchctl() { : }
_ts_write_service /opt/bin/tailscale-socks
cat $TS_SOCKS_PLIST
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"<string>/opt/bin/tailscale-socks</string>"* ]]
  [[ "$output" == *"<string>run</string>"* ]]
  [[ "$output" == *"<key>Label</key>"* ]]
}

@test "darwin restarts on a crash but stays down after a clean exit" {
  run zsh_run darwin24 <<'ZSH'
launchctl() { : }
_ts_write_service /opt/bin/tailscale-socks
cat $TS_SOCKS_PLIST
ZSH
  [[ "$output" == *"<key>SuccessfulExit</key>"* ]]
  [[ "$output" == *"<false/>"* ]]
}

@test "darwin writes a plist macOS itself accepts" {
  run zsh_run darwin24 <<'ZSH'
launchctl() { : }
_ts_write_service /opt/bin/tailscale-socks
print -r -- $TS_SOCKS_PLIST
ZSH
  [ "$status" -eq 0 ]
  if command -v plutil >/dev/null; then
    run plutil -lint "$HOME/Library/LaunchAgents/tailscale-socks.plist"
    [ "$status" -eq 0 ]
  else
    skip "plutil is macOS only"
  fi
}

@test "darwin escapes XML metacharacters in the paths it writes into the plist" {
  export HOME="$BATS_TEST_TMPDIR/R&D"
  mkdir -p "$HOME"
  run zsh_run darwin <<'ZSH'
launchctl() { : }
_ts_write_service '/opt/R&D/tailscale-socks'
cat $TS_SOCKS_PLIST
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"<string>/opt/R&amp;D/tailscale-socks</string>"* ]]
  [[ "$output" != *"/opt/R&D/"* ]]
  if command -v plutil >/dev/null; then
    run plutil -lint "$HOME/Library/LaunchAgents/tailscale-socks.plist"
    [ "$status" -eq 0 ]
  fi
}

@test "darwin reloads the agent instead of stacking a second one" {
  run zsh_run darwin24 <<'ZSH'
launchctl() { print -r -- "launchctl $*" }
_ts_write_service /opt/bin/tailscale-socks
ZSH
  [[ "${lines[0]}" == "launchctl bootout gui/"*"/tailscale-socks" ]]
  [[ "${lines[1]}" == "launchctl bootstrap gui/"* ]]
}

@test "darwin reads the pid out of launchctl print" {
  run zsh_run darwin24 <<'ZSH'
launchctl() { print -l "  state = running" "  pid = 55012" "  program = /opt/bin/x" }
_ts_pid
ZSH
  [ "$output" = "55012" ]
}

# --- Linux: systemd ---------------------------------------------------------

@test "linux writes a unit that runs the resolved binary" {
  run zsh_run linux-gnu <<'ZSH'
systemctl() { : }
_ts_write_service /opt/bin/tailscale-socks
cat $TS_SOCKS_UNIT
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"ExecStart=/opt/bin/tailscale-socks run"* ]]
  [[ "$output" == *"Restart=on-failure"* ]]
  [[ "$output" == *"WantedBy=default.target"* ]]
}

@test "linux doubles a percent sign, which systemd would read as a specifier" {
  isolate
  run zsh_run linux <<'ZSH'
systemctl() { : }
_ts_write_service '/opt/100%pure/tailscale-socks'
cat $TS_SOCKS_UNIT
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"ExecStart=/opt/100%%pure/tailscale-socks run"* ]]
}

@test "linux enables the unit for autostart" {
  run zsh_run linux-gnu <<'ZSH'
systemctl() { print -r -- "systemctl $*" }
_ts_write_service /opt/bin/tailscale-socks
ZSH
  [[ "$output" == *"systemctl --user daemon-reload"* ]]
  [[ "$output" == *"systemctl --user enable tailscale-socks"* ]]
}

@test "linux treats MainPID 0 as stopped" {
  run zsh_run linux-gnu <<'ZSH'
systemctl() { print -r -- 0 }
print -r -- "[$(_ts_pid)]"
ZSH
  [ "$output" = "[]" ]
}

@test "linux reports a real MainPID" {
  run zsh_run linux-gnu <<'ZSH'
systemctl() { print -r -- 4242 }
_ts_pid
ZSH
  [ "$output" = "4242" ]
}

@test "linux refuses to install without a systemd user session" {
  stub_exe systemctl 'exit 1'
  run zsh_run linux-gnu <<'ZSH'
_ts_svc_check
ZSH
  [ "$status" -ne 0 ]
  [[ "$output" == *"no systemd user session"* ]]
}

@test "linux accepts a working systemd user session" {
  stub_exe systemctl 'exit 0'
  run zsh_run linux-gnu <<'ZSH'
_ts_svc_check && print -r -- ok
ZSH
  [ "$output" = "ok" ]
}

# --- Windows: Task Scheduler ------------------------------------------------
#
# cygpath and powershell are shell functions here, so the whole backend runs
# on any machine. The wrapper and the registration command are the artefacts
# that decide whether the service works, so both are asserted in full.

win() {
  run zsh_run msys <<'ZSH'
cygpath()    { print -rn -- "C:${3//\//\\}" }
powershell() { print -r -- "$4" > $HOME/ps.log }
_ts_write_service $HOME/bin/tailscale-socks.exe || exit 1
print -r -- "--- run.cmd ---"
cat -v $TS_SOCKS_CMD
print -r -- "--- powershell ---"
cat $HOME/ps.log
ZSH
}

@test "windows writes a wrapper that appends stdout and stderr to the log" {
  win
  [ "$status" -eq 0 ]
  [[ "$output" == *'"C:'*'tailscale-socks.exe" run >> "C:'*'tailscale-socks.log" 2>&1'* ]]
}

@test "windows ends every wrapper line with CRLF, which cmd.exe needs" {
  run zsh_run msys <<'ZSH'
cygpath()    { print -rn -- "C:${3//\//\\}" }
powershell() { : }
_ts_write_service $HOME/bin/tailscale-socks.exe || exit 1
# Counted, not sampled: one bare LF anywhere is enough for cmd.exe to
# mis-parse the file, so a check on the first line proves nothing.
print -r -- "$(grep -c '' $TS_SOCKS_CMD) $(cat -v $TS_SOCKS_CMD | grep -c '\^M$')"
ZSH
  [ "$status" -eq 0 ]
  total=${output% *}; crlf=${output#* }
  [ "$total" -ge 2 ]
  [ "$total" -eq "$crlf" ]
}

@test "windows registers a task that starts at logon" {
  win
  [[ "$output" == *"Register-ScheduledTask -TaskName 'tailscale-socks'"* ]]
  [[ "$output" == *"New-ScheduledTaskTrigger -AtLogOn"* ]]
}

@test "windows overrides the battery defaults that would keep it from running" {
  win
  [[ "$output" == *"-AllowStartIfOnBatteries"* ]]
  [[ "$output" == *"-DontStopIfGoingOnBatteries"* ]]
}

@test "windows removes the 72 hour execution limit" {
  win
  [[ "$output" == *"-ExecutionTimeLimit ([TimeSpan]::Zero)"* ]]
}

@test "windows restarts on failure and refuses a second instance" {
  win
  [[ "$output" == *"-RestartCount 3"* ]]
  [[ "$output" == *"-MultipleInstances IgnoreNew"* ]]
}

@test "windows registers the wrapper, never the binary directly" {
  win
  [[ "$output" == *"New-ScheduledTaskAction -Execute 'C:"*"run.cmd'"* ]]
}

@test "windows doubles an apostrophe in the path it hands to PowerShell" {
  export HOME="$BATS_TEST_TMPDIR/O'Brien"
  mkdir -p "$HOME"
  run zsh_run msys <<'ZSH'
cygpath()    { print -rn -- "C:${3//\//\\}" }
powershell() { print -r -- "$4" }
_ts_write_service /opt/bin/tailscale-socks.exe
ZSH
  [ "$status" -eq 0 ]
  [[ "$output" == *"O''Brien"* ]]
  [[ "$output" != *"O'Brien"* ]]
}

@test "windows stops before registering when the wrapper cannot be written" {
  run zsh_run msys <<'ZSH'
cygpath()    { print -rn -- "C:${3//\//\\}" }
powershell() { print -r -- "REGISTERED" }
mkdir -p $TS_SOCKS_DIR
# A directory where run.cmd goes: the redirect fails, and nothing may be
# registered against a wrapper that does not exist.
rm -f $TS_SOCKS_CMD; mkdir -p $TS_SOCKS_CMD
_ts_write_service /opt/bin/tailscale-socks.exe && print -r -- "returned success"
ZSH
  [[ "$output" != *"REGISTERED"* ]]
  [[ "$output" != *"returned success"* ]]
}

@test "windows uses LOCALAPPDATA when Windows provides it" {
  export TS_TEST_LOCALAPPDATA='C:\Users\x\AppData\Local'
  stub_exe cygpath 'printf /c/Users/x/AppData/Local'
  run zsh_run msys <<'ZSH'
print -r -- $TS_SOCKS_DIR
ZSH
  [ "$output" = "/c/Users/x/AppData/Local/tailscale-socks" ]
}

@test "windows falls back to \$HOME without LOCALAPPDATA" {
  run zsh_run msys <<'ZSH'
print -r -- ${TS_SOCKS_DIR#$HOME/}
ZSH
  [ "$output" = "tailscale-socks" ]
}

@test "windows looks up the pid by image name, without the .exe" {
  export TS_SOCKS_BIN="$BATS_TEST_TMPDIR/bin/tailscale-socks"
  run zsh_run msys <<'ZSH'
powershell() { print -r -- "$4" }
_ts_pid
ZSH
  [[ "$output" == *"Get-Process -Name 'tailscale-socks'"* ]]
  [[ "$output" != *".exe"* ]]
}
