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
make vuln           # govulncheck over the dependency tree (needs network)
make release        # static binaries for linux/darwin/windows into dist/
```

## Run

```sh
export TS_AUTHKEY=tskey-auth-...       # optional; otherwise a login URL is printed
tailscale-socks --exit-node auto
```

Two commands, `run` (the default, so it can be omitted) and `status`:

```sh
tailscale-socks                        # same as: tailscale-socks run
tailscale-socks status                 # join, print the summary, exit
```

Both print a summary of what the node can reach:

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
Makefile              build, check, vuln, release
```

Dependencies: [`tailscale.com`](https://tailscale.com) (tsnet),
[`alecthomas/kong`](https://github.com/alecthomas/kong) (CLI),
[`things-go/go-socks5`](https://github.com/things-go/go-socks5),
[`joho/godotenv`](https://github.com/joho/godotenv), `golang.org/x/net` (DNS
message parsing).
