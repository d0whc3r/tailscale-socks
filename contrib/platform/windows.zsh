# tailscale-socks — Windows backend: a Task Scheduler task, run at logon.
#
# zsh on Windows means MSYS2, Cygwin or Git Bash, so cygpath translates the
# paths and powershell.exe drives the scheduler. schtasks would do too, but
# only PowerShell can set the task options this service needs.
#
# The node runs under a cmd.exe wrapper, so a console window stays open while
# it does. Task Scheduler cannot redirect output on its own, and hiding the
# window means an S4U task, which fails outright on an account without the
# batch logon right.

TS_SOCKS_DIR=${${LOCALAPPDATA:+$(cygpath -u -- $LOCALAPPDATA 2>/dev/null)}:-$HOME}/$TS_SOCKS_LABEL
TS_SOCKS_CMD=$TS_SOCKS_DIR/run.cmd
TS_SOCKS_LOG=$TS_SOCKS_DIR/$TS_SOCKS_LABEL.log

# _ts_ps runs one PowerShell command. MSYS2 and Git Bash rewrite any argument
# that looks like a POSIX path, turning -Command into a C:\ path; both honour
# these two variables, and Cygwin never rewrites.
_ts_ps() {
  MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' \
    powershell -NoProfile -NonInteractive -Command $1
}

_ts_svc_check() {
  (( $+commands[powershell] && $+commands[cygpath] )) && return 0
  print -u2 "tailscale-socks: powershell and cygpath are required; run this from MSYS2, Cygwin or Git Bash"
  return 1
}

_ts_installed() {
  _ts_ps "if (-not (Get-ScheduledTask -TaskName '$TS_SOCKS_LABEL' -ErrorAction SilentlyContinue)) { exit 1 }" >/dev/null 2>&1
}

_ts_write_service() {
  local bin=$1 binwin logwin cmdwin cmdps settings
  mkdir -p $TS_SOCKS_DIR || return 1
  binwin=$(cygpath -w -- $bin)          || return 1
  logwin=$(cygpath -w -- $TS_SOCKS_LOG) || return 1

  # Task Scheduler runs a program, not a shell, so it cannot redirect; this
  # wrapper is what gives ts_logs and ts_status something to read. CRLF
  # because cmd.exe mis-parses a .cmd file with bare LF line endings.
  printf '@echo off\r\n"%s" run >> "%s" 2>&1\r\n' $binwin $logwin > $TS_SOCKS_CMD || return 1
  cmdwin=$(cygpath -w -- $TS_SOCKS_CMD) || return 1
  # A single quote is the only character PowerShell reads inside single quotes.
  cmdps=${cmdwin//\'/\'\'}

  # Every option here overrides a Task Scheduler default that would break a
  # long-running service: tasks do not start on battery, stop when the machine
  # goes on battery, and are killed after 72 hours. RestartCount is the
  # crash-only restart that launchd and systemd give.
  settings='New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -MultipleInstances IgnoreNew'
  _ts_ps "Register-ScheduledTask -TaskName '$TS_SOCKS_LABEL' -Force -Action (New-ScheduledTaskAction -Execute '$cmdps') -Trigger (New-ScheduledTaskTrigger -AtLogOn) -Settings ($settings) | Out-Null"
}

_ts_remove_service() {
  _ts_stop >/dev/null 2>&1
  _ts_ps "Unregister-ScheduledTask -TaskName '$TS_SOCKS_LABEL' -Confirm:\$false" >/dev/null 2>&1
  rm -f $TS_SOCKS_CMD
}

_ts_start() { _ts_ps "Start-ScheduledTask -TaskName '$TS_SOCKS_LABEL'" }

_ts_stop() {
  _ts_ps "Stop-ScheduledTask -TaskName '$TS_SOCKS_LABEL'" || return 1
  # Stop-ScheduledTask ends the task's own process, the cmd wrapper; the node
  # it started can outlive it. Windows has no SIGTERM for a detached console
  # process, and the state file is written to a temp file and renamed, so a
  # hard stop cannot corrupt the login.
  local pid=$(_ts_pid)
  [[ -n $pid ]] && _ts_ps "Stop-Process -Id $pid -Force" >/dev/null 2>&1
  return 0
}

# The task does not report the node's pid: it started the cmd wrapper, which
# started the node. Match on the image name instead.
_ts_pid() {
  local exe=${${$(_ts_bin 2>/dev/null):t}:r}
  [[ -z $exe ]] && return 1
  _ts_ps "(Get-Process -Name '$exe' -ErrorAction SilentlyContinue | Select-Object -First 1).Id" 2>/dev/null | tr -d '\r'
}

_ts_logtail()   { tail -n $1 $TS_SOCKS_LOG 2>/dev/null }
_ts_logfollow() { tail -n 50 -f $TS_SOCKS_LOG }
_ts_boot_hint() { print "autostart: at logon, by the Task Scheduler task $TS_SOCKS_LABEL" }
