# tailscale-socks — macOS backend: a launchd user agent.

TS_SOCKS_PLIST=$HOME/Library/LaunchAgents/$TS_SOCKS_LABEL.plist
TS_SOCKS_LOG=$HOME/Library/Logs/$TS_SOCKS_LABEL.log

_ts_svc_check() {
  (( $+commands[launchctl] )) && return 0
  print -u2 "tailscale-socks: launchctl not found"
  return 1
}

_ts_installed() { [[ -f $TS_SOCKS_PLIST ]] }

# _ts_xml escapes the three characters that would make the plist invalid XML.
# A path is allowed to contain all of them; launchd just refuses the file.
_ts_xml() {
  local s=${1//&/&amp;}
  s=${s//</&lt;}
  print -rn -- ${s//>/&gt;}
}

_ts_write_service() {
  local bin=$(_ts_xml $1) log=$(_ts_xml $TS_SOCKS_LOG)
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
  <string>$log</string>
  <key>StandardErrorPath</key>
  <string>$log</string>
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
