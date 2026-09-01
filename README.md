# tailscale-socks

A userspace Tailscale node (`tsnet`) that exposes your tailnet to local apps
through three front doors:

| Front door | Default | What it gives you |
|---|---|---|
| SOCKS5 | `127.0.0.1:1080` | TCP into the tailnet, names resolved on the tailnet side |
| HTTP proxy | `127.0.0.1:8080` | `CONNECT` tunnels and plain proxied requests |
| DNS | `127.0.0.1:5354` | MagicDNS, split DNS and exit-node DNS over UDP and TCP |

No `tailscaled`, no root, no TUN device — WireGuard runs in-process. Traffic can
leave through an **exit node** (`--exit-node auto` lets Tailscale pick) and reach
**subnet routers** (on by default).

## Build

Needs Go 1.27 or newer.

```sh
make build          # or: go build -o tailscale-socks ./cmd/tailscale-socks
make check          # gofmt + go vet + staticcheck + go test -race
make cover          # per-function statement coverage; cover-html for a browser
make vuln           # govulncheck over the dependency tree (needs network)
make outdated       # direct dependencies with a newer version (needs network)
make release        # static binaries for linux/darwin/windows into dist/
```

## Run

```sh
export TS_AUTHKEY=tskey-auth-...       # optional; otherwise a login URL is printed
tailscale-socks --exit-node auto
```

Three commands, `run` (the default, so it can be omitted), `status` and
`config`:

```sh
tailscale-socks                        # same as: tailscale-socks run
tailscale-socks status                 # join, print the summary, exit
tailscale-socks config                 # print the settings; no tailnet, no login
```

`run` and `status` print a summary of what the node can reach:

```
node:     my-proxy.tailnet.ts.net (Running)
addrs:    100.64.0.7, fd7a:115c:a1e0::7
tailnet:  example.com (MagicDNS suffix tailnet.ts.net)
state:    /Users/you/Library/Application Support/tailscale-socks/my-proxy
dns:      accept=true
routes:   accept=true
exit node: gateway.tailnet.ts.net online=true
exit node candidates:
  - gateway.tailnet.ts.net (online=true)
subnet routers:
  - office-router.tailnet.ts.net -> 192.168.10.0/24
```

Full help: `tailscale-socks --help`, `tailscale-socks run --help`.

## Flags

Every flag has a one-letter alias and an environment variable. Long names take
`--`, aliases take `-`.

| Flag | Alias | Env | Default | Meaning |
|---|---|---|---|---|
| `--hostname` | `-n` | `TSPROXY_HOSTNAME` | `ts-proxy` | node name on the tailnet |
| `--auth-key` | `-k` | `TS_AUTHKEY` | none | auth key for unattended login |
| `--state-dir` | `-D` | `TSPROXY_STATE_DIR` | see [Login state](#login-state) | where the login is persisted |
| `--socks5` | `-s` | `TSPROXY_SOCKS5` | `127.0.0.1:1080` | SOCKS5 listen address; empty disables |
| `--http` | `-p` | `TSPROXY_HTTP` | `127.0.0.1:8080` | HTTP proxy listen address; empty disables |
| `--dns` | `-d` | `TSPROXY_DNS` | `127.0.0.1:5354` | DNS listen address (UDP+TCP); empty disables |
| `--exit-node` | `-e` | `TSPROXY_EXIT_NODE` | `off` | `auto`, `auto:<expr>`, peer name, Tailscale IP, or `off` |
| `--exit-node-allow-lan` | `-l` | `TSPROXY_EXIT_NODE_ALLOW_LAN` | off | keep the local LAN reachable while an exit node is in use |
| `--accept-routes` | `-r` | `TSPROXY_ACCEPT_ROUTES` | on | accept subnet routes; turn off with `--no-accept-routes` |
| `--accept-dns` | `-a` | `TSPROXY_ACCEPT_DNS` | on | use the tailnet DNS config (required by `--dns`); off with `--no-accept-dns` |
| `--verbose` | `-v` | `TSPROXY_VERBOSE` | off | also log tsnet internals |
| `--version` | `-V` | | | print the version |
| `--help` | `-h` | | | show help |

```sh
tailscale-socks -e auto -s 127.0.0.1:1080 -p '' -d ''
```

Aliases take their value after a space (`-s 127.0.0.1:1080`); the `=` form
(`-s=...`) is a long-flag thing, so use `--socks5=` to pass an empty value.

## Configuration files

Every flag can also come from a `.env` file, read from two places:

1. `.env` in the directory of the executable
2. `~/.tailscale/.env`

The full order is **command line > environment > executable's `.env` >
`~/.tailscale/.env`**. Missing files are skipped; loaded ones are logged.

`.env.example` documents every variable with its default:

```sh
mkdir -p ~/.tailscale
cp .env.example ~/.tailscale/.env
chmod 600 ~/.tailscale/.env
$EDITOR ~/.tailscale/.env
```

Keep these files at `0600`: they can hold `TS_AUTHKEY`. The program warns when
one is readable by other users.

"Next to the executable" is the real binary's directory (symlinks resolved), so
it does not apply to `go run`, which builds into a temporary directory.

Two more variables are read by `tsnet` itself rather than by a flag:
`TS_CONTROL_URL` points at a self-hosted control server (Headscale), and
`TSNET_FORCE_LOGIN=1` makes an auth key apply to an already logged-in node.

### Reading the configuration back

`tailscale-socks config` walks the same chain and prints what it resolved to.
It joins nothing and logs nothing in, so it is cheap to call from a shell:

```sh
$ tailscale-socks config
TSPROXY_HOSTNAME='ts-proxy'
TSPROXY_STATE_DIR='/Users/you/Library/Application Support/tailscale-socks/ts-proxy'
TSPROXY_SOCKS5='127.0.0.1:1080'
TSPROXY_HTTP='127.0.0.1:8080'
TSPROXY_DNS='127.0.0.1:5354'
TSPROXY_EXIT_NODE='off'
TSPROXY_EXIT_NODE_ALLOW_LAN='false'
TSPROXY_ACCEPT_ROUTES='true'
TSPROXY_ACCEPT_DNS='true'
TSPROXY_VERBOSE='false'
```

With a key it prints that value alone, unquoted, ready for `$(...)`:

```sh
$ tailscale-socks config socks5
127.0.0.1:1080
$ curl --socks5-hostname "$(tailscale-socks config socks5)" http://peer.tailnet.ts.net/
$ eval "$(tailscale-socks config)"      # the whole set, quoted for the shell
```

The key is the flag name or its variable — `socks5` and `TSPROXY_SOCKS5` are
the same key. Flags still apply, so `tailscale-socks config -e auto` answers
"what would that run use?". An empty value means a disabled listener. The auth
key is never printed: this output is made to be piped and logged.

## Run it as a service

`contrib/tailscale-socks.zsh` manages a background node through the platform's
own service manager: a **launchd** user agent on macOS, a **systemd** user unit
on Linux. Source it from `~/.zshrc`:

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
| `ts_proxy on\|off` | send this shell's commands through the proxies |

The service runs `tailscale-socks run` with no flags: configuration stays in
`~/.tailscale/.env`, same as any other invocation. Edit it and `ts_restart`.

The binary is taken from `$PATH` (symlinks resolved) at install time; override
with `TS_SOCKS_BIN`. Move the binary and re-run `ts_install`. The login is not
affected: the state directory keys on the hostname, not on the path.

```
macOS   ~/Library/LaunchAgents/tailscale-socks.plist   log: ~/Library/Logs/tailscale-socks.log
Linux   ~/.config/systemd/user/tailscale-socks.service log: journalctl --user -u tailscale-socks
```

Both start at login. Only a crash restarts the node — `ts_down` stays down. On
Linux, `loginctl enable-linger $USER` also starts it at boot, before login.

`ts_status` does **not** run `tailscale-socks status`: that joins the tailnet
with the same state directory and node key as the running service, and one
login cannot back two nodes. It reports the service state, probes each enabled
listener, and prints the summary the running process wrote when it came up:

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

### Sending the shell through the proxy

`ts_proxy on` exports the proxy variables in the current shell, so anything
started from it reaches the tailnet without per-command flags. `ts_proxy off`
unsets them; `ts_proxy` on its own prints what is set. No other shell is
affected.

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

A disabled listener sets no variable, and `ts_proxy on` warns when nothing is
listening on an enabled one. The bypass list keeps local ports local: without
it a request to `127.0.0.1` would leave through the tailnet and arrive at
another machine.

`ssh` and anything else that ignores these variables still needs its own
configuration — for ssh, a `ProxyCommand` through the SOCKS5 port.

## Login state

The login is stored in `tailscaled.state` (file `0600`, directory `0700`) under:

```
<user config dir>/tailscale-socks/<hostname>
# macOS: ~/Library/Application Support/tailscale-socks/ts-proxy
# Linux: ~/.config/tailscale-socks/ts-proxy
```

Override it with `--state-dir`. The path deliberately does **not** depend on the
binary's name — tsnet's own default is `tsnet-<executable name>`, so renaming or
moving the binary would silently lose the login and register a second node.

Consequences worth knowing:

- One login per `--hostname`. Two hostnames are two nodes with two state
  directories; reusing a hostname reuses its login.
- `TS_AUTHKEY` is only used when the node is not logged in yet. Once state
  exists it is ignored (tsnet logs that it is ignoring it), unless
  `TSNET_FORCE_LOGIN=1` is set.
- Delete the directory to force a fresh login; the old node stays in the tailnet
  until you remove it from the admin console.
- The state file holds the node's private keys. Back it up like a secret, or
  not at all.

`tailscale-socks status` prints the directory in use as `state:`.

## Exit nodes

```sh
tailscale-socks --exit-node auto        # best exit node, re-picked automatically
tailscale-socks --exit-node auto:any    # same, explicit expression
tailscale-socks --exit-node auto:geo:us # restrict the automatic pick
tailscale-socks --exit-node gateway     # a specific peer, by MagicDNS base name
tailscale-socks --exit-node 100.64.0.2  # or by Tailscale IP
tailscale-socks --exit-node off         # none (default)
```

`tailscale-socks status` lists the peers that are advertising an exit node.

## Use it

```sh
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
ALL_PROXY=socks5h://127.0.0.1:1080 some-app
```

Use `socks5h://` (curl: `--socks5-hostname`), not `socks5://`: names must be
resolved by the proxy, on the tailnet, not by your host. Once a name reaches the
proxy it is resolved with the tailnet DNS configuration, so MagicDNS names,
split-DNS domains and — when an exit node is in use — that node's resolvers all
work. Public names fall back to tsnet's own resolution if the tailnet DNS has no
answer.

## Not included

This node consumes the tailnet; it does not offer anything to it. There is no
route or exit-node advertising, no Taildrop, no `serve`/`funnel`, and no
Tailscale SSH.

## Security

The proxies and the DNS server have **no authentication**. They bind to
`127.0.0.1` by default; binding them to `0.0.0.0` turns the machine into an open
relay into your tailnet for anyone who can reach the port.

Port 5354 is the DNS default because 53 needs root and 5353 collides with mDNS.

## Layout

```
cmd/tailscale-socks   CLI (kong): flags, help, .env loading, wiring
internal/tsnode       the Tailscale node: prefs, exit node, DNS, dialing
internal/proxy        SOCKS5, HTTP and DNS servers over a tailnet dialer
.env.example          every variable, with its default
contrib/              zsh helpers, launchd agent and systemd unit
Makefile              build, check, cover, vuln, outdated, release
```

Dependencies: [`tailscale.com`](https://tailscale.com) (tsnet),
[`alecthomas/kong`](https://github.com/alecthomas/kong) (CLI),
[`things-go/go-socks5`](https://github.com/things-go/go-socks5),
[`joho/godotenv`](https://github.com/joho/godotenv), `golang.org/x/net` (DNS
message parsing).
