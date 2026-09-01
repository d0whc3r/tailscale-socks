# tailscale-socks

Reach your tailnet from any local app, without installing Tailscale on the
machine.

`tailscale-socks` joins your tailnet as a **userspace** node — WireGuard runs
in-process, so there is no `tailscaled`, no root and no TUN device — and opens
three local front doors onto it:

| Front door | Default | What it gives you |
|---|---|---|
| SOCKS5 | `127.0.0.1:1080` | TCP into the tailnet, names resolved on the tailnet side |
| HTTP proxy | `127.0.0.1:8080` | `CONNECT` tunnels and plain proxied requests |
| DNS | `127.0.0.1:5354` | MagicDNS, split DNS and exit-node DNS, over UDP and TCP |

Outbound traffic can leave through an **exit node** (`--exit-node auto` lets
Tailscale pick), and **subnet routers** are reachable by default.

Good for a work laptop you would rather not enroll, a container or CI job that
needs one tailnet service, or a shell that should talk to the tailnet only when
you say so.

## Install

Prebuilt binaries for macOS, Linux and Windows are attached to each tagged
release. Or build it yourself, with Go 1.27 or newer:

```sh
git clone https://github.com/d0whc3r/tailscale-socks && cd tailscale-socks
make build          # -> ./tailscale-socks
```

Put the binary anywhere on your `$PATH`. It is a single static file with no
runtime dependencies.

## Quick start

```sh
tailscale-socks
```

The first run prints a login URL. Approve it in the browser and the node
appears in your tailnet as `ts-proxy`; the login is saved, so later runs start
straight away. For unattended machines, use an auth key instead:

```sh
export TS_AUTHKEY=tskey-auth-...
tailscale-socks --exit-node auto
```

Then send something through it:

```sh
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
ALL_PROXY=socks5h://127.0.0.1:1080 some-app
```

Use `socks5h://` (curl: `--socks5-hostname`), **not** `socks5://`. Names must be
resolved by the proxy, on the tailnet, not by your host. Once a name reaches the
proxy it is resolved with the tailnet DNS configuration, so MagicDNS names,
split-DNS domains and — when an exit node is in use — that node's resolvers all
work. Public names fall back to normal resolution when the tailnet DNS has no
answer.

## Commands

```sh
tailscale-socks                 # run the proxies (the default command)
tailscale-socks run             # the same thing, spelled out
tailscale-socks status          # join, print what this node can reach, exit
tailscale-socks config          # print the resolved settings; no tailnet, no login
```

`run` and `status` print the same summary:

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

## Configuration

Settings come from, in decreasing priority:

1. the command line
2. the environment
3. `.env` next to the binary
4. `~/.tailscale/.env`

### Flags

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

Every listener is optional; an empty address disables it. All three empty is an
error.

```sh
tailscale-socks -e auto -s 127.0.0.1:1080 -p '' -d ''    # SOCKS5 only
```

Aliases take their value after a space (`-s 127.0.0.1:1080`); the `=` form
(`-s=...`) is a long-flag thing, so use `--socks5=` to pass an empty value.

Two more variables are read by Tailscale itself rather than by a flag:
`TS_CONTROL_URL` points at a self-hosted control server (Headscale), and
`TSNET_FORCE_LOGIN=1` makes an auth key apply to an already logged-in node.

### `.env` files

`.env.example` documents every variable with its default:

```sh
mkdir -p ~/.tailscale
cp .env.example ~/.tailscale/.env
chmod 600 ~/.tailscale/.env
$EDITOR ~/.tailscale/.env
```

Keep these files at `0600`: they can hold `TS_AUTHKEY`. The program warns when
one is readable by other users. Missing files are skipped; loaded ones are
logged.

"Next to the binary" is the real binary's directory, symlinks resolved — so it
does not apply to `go run`, which builds into a temporary directory.

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

With a key it prints that one value, unquoted, ready for `$(...)`:

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

## Login state

The login is stored in `tailscaled.state` (file `0600`, directory `0700`) under:

```
<user config dir>/tailscale-socks/<hostname>
# macOS: ~/Library/Application Support/tailscale-socks/ts-proxy
# Linux: ~/.config/tailscale-socks/ts-proxy
```

Override it with `--state-dir`. The path deliberately does **not** depend on the
binary's name, so renaming or moving the binary keeps the login.

Worth knowing:

- One login per `--hostname`. Two hostnames are two nodes with two state
  directories; reusing a hostname reuses its login.
- `TS_AUTHKEY` is only used when the node is not logged in yet. Once state
  exists it is ignored, unless `TSNET_FORCE_LOGIN=1` is set.
- Delete the directory to force a fresh login; the old node stays in the tailnet
  until you remove it from the admin console.
- The state file holds the node's private keys. Back it up like a secret, or
  not at all.

`tailscale-socks status` prints the directory in use as `state:`.

## Run it in the background

`contrib/tailscale-socks.zsh` installs the node as a **launchd** user agent on
macOS or a **systemd** user unit on Linux, and adds a few shell functions:

```sh
source /path/to/tailscale-socks/contrib/tailscale-socks.zsh
ts_install          # write the service, enable autostart, start it
ts_status           # service state, listener probes, last node summary
ts_proxy on         # send this shell's commands through the proxies
```

See [docs/service.md](docs/service.md) for the full reference.

## Security

The proxies and the DNS server have **no authentication**. They bind to
`127.0.0.1` by default; binding them to `0.0.0.0` turns the machine into an open
relay into your tailnet for anyone who can reach the port.

The auth key and the state file are credentials: keep `.env` files at `0600`,
and treat the state directory as a secret. Neither is ever printed.

Port 5354 is the DNS default because 53 needs root and 5353 collides with mDNS.

## Not included

This node consumes the tailnet; it does not offer anything to it. There is no
route or exit-node advertising, no Taildrop, no `serve`/`funnel`, and no
Tailscale SSH.

## Documentation

- [docs/service.md](docs/service.md) — running it as a background service
- [docs/architecture.md](docs/architecture.md) — how the program is put together
- [CONTRIBUTING.md](CONTRIBUTING.md) — building, testing and changing it
- [.env.example](.env.example) — every variable, with its default
