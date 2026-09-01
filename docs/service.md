# Running it as a service

`contrib/tailscale-socks.zsh` keeps a `tailscale-socks` node running in the background through the platform's own service manager: a **launchd** user agent on macOS, a **systemd** user unit on Linux, a **Task Scheduler** task on Windows. The installer puts the line in `~/.zshrc` for you; by hand it is:

```sh
source /path/to/tailscale-socks/contrib/tailscale-socks.zsh
```

| Function | What it does |
|---|---|
| `ts_install` | write the service, enable it for autostart, start it |
| `ts_up` | start it |
| `ts_down` | stop it |
| `ts_restart` | stop, start |
| `ts_status` | service state, listener probes, last node summary |
| `ts_logs [-f\|N]` | read the log |
| `ts_uninstall` | stop it and remove the service |
| `ts_proxy on\|off\|show` | send this shell's commands through the proxies |

The service runs `tailscale-socks run` with no flags: configuration stays in `~/.tailscale/.env`, same as any other invocation. Edit it and `ts_restart`.

The binary is taken from `$PATH` (symlinks resolved) at install time; override with `TS_SOCKS_BIN`. Move the binary and re-run `ts_install`. The login is not affected: the state directory keys on the hostname, not on the path.

```
macOS   ~/Library/LaunchAgents/tailscale-socks.plist   log: ~/Library/Logs/tailscale-socks.log
Linux   ~/.config/systemd/user/tailscale-socks.service log: journalctl --user -u tailscale-socks
Windows Task Scheduler task "tailscale-socks"          log: %LOCALAPPDATA%\tailscale-socks\tailscale-socks.log
```

All three start at login. Only a crash restarts the node — `ts_down` stays down. On Linux, `loginctl enable-linger $USER` also starts it at boot, before login.

## Platforms

The backend is picked from `$OSTYPE` and lives in its own file under `contrib/platform/`; the main script sources the one that matches, so the directory has to travel with it. An unknown `$OSTYPE` refuses to load rather than guessing.

| `$OSTYPE` | Backend | Notes |
|---|---|---|
| `darwin*` | launchd user agent | |
| `linux*` | systemd user unit | WSL included, when it has systemd |
| `msys*`, `cygwin*` | Task Scheduler task | zsh on Windows: MSYS2, Cygwin, Git Bash |

`systemctl` being installed is not the same as it working: WSL without `systemd=true` and most containers have the binary but no user session bus. `ts_install` checks for one and says so, instead of leaving you with systemd's `Failed to connect to bus`.

On Windows the task is registered through PowerShell, not `schtasks`, because the `schtasks /SC ONLOGON` defaults break a long-running service: the task will not start on battery, stops when the machine goes on battery, and is killed after 72 hours. Two things differ from the other platforms:

- Task Scheduler cannot redirect output, so the task runs a small `run.cmd` wrapper that appends to the log. A console window stays open while the node runs. Hiding it means an S4U task, which fails outright on an account without the batch logon right.
- Windows has no SIGTERM for a detached console process, so `ts_down` is a hard stop. The state file is written to a temp file and renamed, so this cannot corrupt the login.

## Status

`ts_status` does **not** run `tailscale-socks status`: that joins the tailnet with the same state directory and node key as the running service, and one login cannot back two nodes. It reports the service state, probes each enabled listener, and prints the summary the running process wrote when it came up:

```
service:  running (pid 55012)
socks5:   127.0.0.1:1080 up
http:     127.0.0.1:8080 up
dns:      disabled

node:     my-proxy.tailnet.ts.net (Running)
...
exit node: gateway.tailnet.ts.net online=true
```

For a live summary, stop the service first: `ts_down && tailscale-socks status`.

## Sending the shell through the proxy

`ts_proxy on` exports the proxy variables in the current shell, so anything started from it reaches the tailnet without per-command flags. `ts_proxy off` unsets them; `ts_proxy` on its own prints what is set. No other shell is affected.

```sh
ts_proxy on
curl http://peer.tailnet.ts.net/      # no --proxy, no --socks5-hostname
ts_proxy off
```

| Variable | Value |
|---|---|
| `http_proxy`, `https_proxy` (and the uppercase pair) | `http://` + the `--http` address |
| `ALL_PROXY`, `all_proxy` | `socks5h://` + the `--socks5` address |
| `NO_PROXY`, `no_proxy` | `localhost,127.0.0.1,::1` |

The addresses come from `tailscale-socks config`, so the shell never parses a configuration file itself. A disabled listener sets no variable, and `ts_proxy on` warns when nothing is listening on an enabled one.

The bypass list keeps local ports local: without it a request to `127.0.0.1` would leave through the tailnet and arrive at another machine.

`ssh` and anything else that ignores these variables still needs its own configuration — for ssh, a `ProxyCommand` through the SOCKS5 port.
