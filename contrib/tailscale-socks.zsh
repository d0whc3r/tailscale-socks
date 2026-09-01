# tailscale-socks — zsh helpers.
#
# Source it from ~/.zshrc:
#
#     source /path/to/tailscale-socks/contrib/tailscale-socks.zsh
#
#   ts_install     write and load the service (launchd agent / systemd user unit)
#   ts_up          start it
#   ts_down        stop it
#   ts_restart     stop, start
#   ts_status      service state, listeners, last node summary
#   ts_logs [-f]   read the log
#   ts_uninstall   stop it and remove the service
#   ts_proxy       point this shell's commands at the proxies: on, off, show
#
# The binary is found on $PATH; override with TS_SOCKS_BIN. Flags are not
# passed here: the service reads ~/.tailscale/.env, like every other
# invocation.

TS_SOCKS_LABEL=tailscale-socks
TS_SOCKS_PLIST=$HOME/Library/LaunchAgents/$TS_SOCKS_LABEL.plist
TS_SOCKS_UNIT=$HOME/.config/systemd/user/$TS_SOCKS_LABEL.service
TS_SOCKS_LOG=$HOME/Library/Logs/$TS_SOCKS_LABEL.log

# --- helpers ----------------------------------------------------------------

# _ts_bin prints the absolute path of the binary, symlinks resolved: the
# service must not depend on a $PATH that launchd or systemd may not have.
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
# keeps this free of nc, which is not the same program on both platforms.
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

# --- platform ---------------------------------------------------------------

if [[ $OSTYPE == darwin* ]]; then

  _ts_installed() { [[ -f $TS_SOCKS_PLIST ]] }

  _ts_write_service() {
    local bin=$1
    mkdir -p ${TS_SOCKS_PLIST:h} ${TS_SOCKS_LOG:h}
    cat > $TS_SOCKS_PLIST <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$TS_SOCKS_LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$bin</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <!-- Restart on a crash only. A clean exit (SIGTERM from ts_down) stays down. -->
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>StandardOutPath</key>
  <string>$TS_SOCKS_LOG</string>
  <key>StandardErrorPath</key>
  <string>$TS_SOCKS_LOG</string>
</dict>
</plist>
PLIST
    launchctl bootout gui/$UID/$TS_SOCKS_LABEL 2>/dev/null
    launchctl bootstrap gui/$UID $TS_SOCKS_PLIST
  }

  _ts_remove_service() {
    launchctl bootout gui/$UID/$TS_SOCKS_LABEL 2>/dev/null
    rm -f $TS_SOCKS_PLIST
  }

  _ts_start()   { launchctl kickstart gui/$UID/$TS_SOCKS_LABEL }
  _ts_stop()    { launchctl kill TERM gui/$UID/$TS_SOCKS_LABEL }
  _ts_pid()     { launchctl print gui/$UID/$TS_SOCKS_LABEL 2>/dev/null | sed -n 's/^[[:space:]]*pid = \([0-9]*\).*/\1/p' }
  _ts_logtail() { tail -n $1 $TS_SOCKS_LOG 2>/dev/null }
  _ts_logfollow() { tail -n 50 -f $TS_SOCKS_LOG }
  _ts_boot_hint() { print "autostart: at login, by ~/Library/LaunchAgents" }

else

  _ts_installed() { [[ -f $TS_SOCKS_UNIT ]] }

  _ts_write_service() {
    local bin=$1
    mkdir -p ${TS_SOCKS_UNIT:h}
    cat > $TS_SOCKS_UNIT <<UNIT
[Unit]
Description=tailscale-socks: SOCKS5, HTTP and DNS front doors to a tailnet
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$bin run
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
UNIT
    systemctl --user daemon-reload
    systemctl --user enable $TS_SOCKS_LABEL
  }

  _ts_remove_service() {
    systemctl --user disable --now $TS_SOCKS_LABEL 2>/dev/null
    rm -f $TS_SOCKS_UNIT
    systemctl --user daemon-reload
  }

  _ts_start()   { systemctl --user start $TS_SOCKS_LABEL }
  _ts_stop()    { systemctl --user stop $TS_SOCKS_LABEL }
  _ts_pid()     { systemctl --user show -p MainPID --value $TS_SOCKS_LABEL 2>/dev/null | grep -v '^0$' }
  _ts_logtail() { journalctl --user -u $TS_SOCKS_LABEL -n $1 --no-pager -o cat 2>/dev/null }
  _ts_logfollow() { journalctl --user -u $TS_SOCKS_LABEL -n 50 -f -o cat }
  _ts_boot_hint() {
    print "autostart: at login; for boot without login run: loginctl enable-linger $USER"
  }

fi

# --- commands ---------------------------------------------------------------

ts_install() {
  local bin
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
