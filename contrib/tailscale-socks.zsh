# tailscale-socks — zsh helpers.
#
# Source it from ~/.zshrc:
#
#     source /path/to/tailscale-socks/contrib/tailscale-socks.zsh
#
#   ts_install     write and load the service (launchd / systemd / Task Scheduler)
#   ts_up          start it
#   ts_down        stop it
#   ts_restart     stop, start
#   ts_status      service state, listeners, last node summary
#   ts_logs [-f]   read the log
#   ts_uninstall   stop it and remove the service
#   ts_proxy       point this shell's commands at the proxies: on, off, show
#
# Tab completion for those and for the binary is registered too, when compinit
# has already run: keep the source line below your compinit.
#
# The binary is found on $PATH; override with TS_SOCKS_BIN. Flags are not
# passed here: the service reads ~/.tailscale/.env, like every other
# invocation.

TS_SOCKS_LABEL=tailscale-socks

# --- platform ---------------------------------------------------------------
#
# One file per service manager, under platform/. Each defines the same set:
#
#   _ts_svc_check          the service manager is present and usable here
#   _ts_installed          the service is written
#   _ts_write_service BIN  write it and enable it for autostart
#   _ts_remove_service     stop it and remove it
#   _ts_start _ts_stop     run it, end it
#   _ts_pid                the node's pid, empty when it is not running
#   _ts_logtail N          the last N lines
#   _ts_logfollow          follow the log
#   _ts_boot_hint          one line on what starts it
#
# zsh on Windows is MSYS2, Cygwin or Git Bash; WSL reports itself as Linux,
# which is right — it runs the systemd unit when it has a systemd to run it.

case $OSTYPE in
  darwin*)       TS_SOCKS_OS=darwin  ;;
  linux*)        TS_SOCKS_OS=linux   ;;
  msys*|cygwin*) TS_SOCKS_OS=windows ;;
  *)
    print -u2 "tailscale-socks: no service backend for OSTYPE=$OSTYPE"
    return 1
    ;;
esac

# %x is this file even when sourced, which $0 is not under every option set.
source ${${(%):-%x}:A:h}/platform/$TS_SOCKS_OS.zsh || return 1

# --- helpers ----------------------------------------------------------------

# _ts_bin prints the absolute path of the binary, symlinks resolved: the
# service must not depend on a $PATH that the service manager may not have.
_ts_bin() {
  local b=${TS_SOCKS_BIN:-${commands[tailscale-socks]}}
  if [[ ! -x $b ]]; then
    print -u2 "tailscale-socks: binary not found; set TS_SOCKS_BIN=/path/to/tailscale-socks"
    return 1
  fi
  print -r -- ${b:A}
}

# _ts_cfg SETTING prints one resolved value. The binary owns the precedence —
# command line, environment, .env by the binary, ~/.tailscale/.env — so
# nothing here parses a configuration file.
_ts_cfg() {
  local bin
  bin=$(_ts_bin) || return
  # stderr carries "config: loaded ..."; the caller only wants the value.
  $bin config $1 2>/dev/null
}

# _ts_probe ADDR succeeds when something accepts TCP on ADDR. zsh/net/tcp
# keeps this free of nc, which is not the same program on every platform.
_ts_probe() {
  local host=${1%:*} port=${1##*:}
  host=${host#\[}; host=${host%\]}
  [[ -z $host ]] && host=127.0.0.1
  zmodload -F zsh/net/tcp b:ztcp 2>/dev/null || return 2
  ztcp $host $port 2>/dev/null || return 1
  ztcp -c $REPLY
}

# _ts_summary prints the node summary the last run wrote at startup: the lines
# from "node:" up to the next timestamped log line.
_ts_summary() {
  _ts_logtail 500 | awk '
    /^node: /                      { b = $0 ORS; c = 1; next }
    c && /^[0-9][0-9][0-9][0-9]\// { c = 0 }
    c                              { b = b $0 ORS }
    END                            { printf "%s", b }'
}

# --- commands ---------------------------------------------------------------

ts_install() {
  local bin
  _ts_svc_check || return 1
  bin=$(_ts_bin) || return 1
  _ts_write_service $bin || return 1
  print "installed: $bin"
  _ts_boot_hint
  ts_up
}

ts_up() {
  _ts_installed || { print -u2 "tailscale-socks: not installed; run ts_install"; return 1 }
  _ts_start || return 1
  print "started"
}

ts_down() {
  _ts_installed || { print -u2 "tailscale-socks: not installed"; return 1 }
  _ts_stop || return 1
  print "stopped"
}

ts_restart() { ts_down && ts_up }

ts_uninstall() {
  _ts_installed || { print "not installed"; return 0 }
  _ts_remove_service
  print "removed"
}

# ts_status reports on the running service. It deliberately does not run
# `tailscale-socks status`: that joins the tailnet with the same state
# directory and node key as the service, and two nodes cannot share one login.
# The summary below is what the running process printed when it came up.
ts_status() {
  local pid addr name
  if ! _ts_installed; then
    print "service:  not installed"
    return 1
  fi
  pid=$(_ts_pid)
  if [[ -z $pid ]]; then
    print "service:  stopped"
    return 1
  fi
  print "service:  running (pid $pid)"

  for name in socks5 http dns; do
    addr=$(_ts_cfg $name)
    if [[ -z $addr ]]; then
      printf '%-9s disabled\n' "$name:"
    elif _ts_probe $addr; then
      printf '%-9s %s up\n' "$name:" "$addr"
    else
      printf '%-9s %s NOT LISTENING\n' "$name:" "$addr"
    fi
  done

  local summary=$(_ts_summary)
  if [[ -n $summary ]]; then
    print
    print -r -- $summary
  fi
}

ts_logs() {
  if [[ $1 == -f ]]; then
    _ts_logfollow
  else
    _ts_logtail ${1:-50}
  fi
}

# ts_proxy points this shell at the proxies, so every command started from it
# goes through the tailnet. "on" exports the variables, "off" unsets them, no
# argument prints what is set. Only this shell is affected.
ts_proxy() {
  local socks http addr
  case ${1:-show} in
    on)
      socks=$(_ts_cfg socks5)
      http=$(_ts_cfg http)
      if [[ -z $socks && -z $http ]]; then
        print -u2 "tailscale-socks: no proxy listener is enabled"
        return 1
      fi
      # Start from off: a listener disabled since the last call must not
      # leave its variable behind.
      unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy
      if [[ -n $http ]]; then
        export http_proxy=http://$http https_proxy=http://$http
        export HTTP_PROXY=$http_proxy HTTPS_PROXY=$https_proxy
      fi
      # socks5h, not socks5: names must be resolved by the proxy, on the
      # tailnet, or MagicDNS names do not exist for the client.
      if [[ -n $socks ]]; then
        export ALL_PROXY=socks5h://$socks all_proxy=socks5h://$socks
      fi
      # Without this, a request to a local port would go out through the
      # tailnet and come back to a different machine.
      export NO_PROXY=localhost,127.0.0.1,::1 no_proxy=localhost,127.0.0.1,::1
      for addr in $http $socks; do
        _ts_probe $addr || print -u2 "warning: nothing is listening on $addr (see ts_status)"
      done
      ts_proxy show
      ;;
    off)
      unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy NO_PROXY no_proxy
      print "proxy:    off"
      ;;
    show)
      if [[ -z $http_proxy && -z $ALL_PROXY ]]; then
        print "proxy:    off"
        return 1
      fi
      [[ -n $http_proxy ]] && printf '%-9s %s\n' "http:" "$http_proxy"
      [[ -n $ALL_PROXY ]] && printf '%-9s %s\n' "socks5:" "$ALL_PROXY"
      printf '%-9s %s\n' "bypass:" "$NO_PROXY"
      ;;
    *)
      print -u2 "usage: ts_proxy [on|off|show]"
      return 2
      ;;
  esac
}

# --- completion -------------------------------------------------------------
#
# The candidates come from the binary's own help and `config` output, so a new
# flag or setting completes without a second list to keep in step here.

# _ts_flags COMMAND prints the long flags of one subcommand. Kong prints them
# one per line under "Flags:", as "-s, --socks5=ADDR", and a negatable one as
# "--[no-]accept-dns", which stands for two flags.
_ts_flags() {
  local bin
  bin=$(_ts_bin 2>/dev/null) || return
  $bin $1 --help 2>&1 | awk '
    /^Flags:/ { flags = 1; next }
    !flags    { next }
    {
      for (i = 1; i <= 2; i++) if ($i ~ /^--/) {
        split($i, part, "=")
        name = part[1]
        if (name ~ /^--\[no-\]/) {
          sub(/\[no-\]/, "", name); print name
          sub(/^--/, "--no-", name)
        }
        print name
      }
    }' | sort -u
}

# _ts_settings prints the name of every setting `config` answers to.
_ts_settings() {
  local bin env
  bin=$(_ts_bin 2>/dev/null) || return
  $bin config 2>/dev/null | while IFS='=' read -r env _; do
    print -r -- ${${${(L)env}#tsproxy_}//_/-}
  done
}

_tailscale_socks() {
  local cmd=$words[2]
  local -a matches
  if [[ $words[CURRENT] == -* ]]; then
    # run is the default command, so its flags are valid without naming it.
    [[ $cmd == (run|status|config|upgrade) ]] || cmd=run
    matches=( ${(f)"$(_ts_flags $cmd)"} )
  elif (( CURRENT == 2 )); then
    matches=( run status config upgrade )
  elif [[ $cmd == config ]]; then
    matches=( ${(f)"$(_ts_settings)"} )
  fi
  (( $#matches )) && compadd -a matches
}

_ts_proxy() { compadd on off show }
_ts_logs()  { compadd -- -f }

# _ts_compdefs registers the completions above. compdef exists only once
# compinit has run, and the source line in ~/.zshrc may sit above it: skipping
# is the whole recovery, the functions still work, only the Tab does not.
_ts_compdefs() {
  compdef _tailscale_socks tailscale-socks
  compdef _ts_proxy ts_proxy
  compdef _ts_logs ts_logs
  compdef _nothing ts_install ts_up ts_down ts_restart ts_status ts_uninstall
}

# An `if`, not `&&`: the last status is what `source` returns, and a shell
# without compdef must not make sourcing this file look like a failure.
if (( $+functions[compdef] )); then
  _ts_compdefs
fi
