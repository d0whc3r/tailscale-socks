# tailscale-socks — Linux backend: a systemd user unit.

TS_SOCKS_UNIT=$HOME/.config/systemd/user/$TS_SOCKS_LABEL.service

# systemctl being installed is not the same as it working: WSL without
# systemd=true and most containers have the binary but no user session bus, and
# every call there fails with "Failed to connect to bus".
_ts_svc_check() {
  (( $+commands[systemctl] )) && systemctl --user show -p Version >/dev/null 2>&1 && return 0
  print -u2 "tailscale-socks: no systemd user session here (WSL without systemd, or a container)."
  print -u2 "                 run 'tailscale-socks run' yourself, or start it from your own init."
  return 1
}

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
