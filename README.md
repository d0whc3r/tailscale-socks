# tailscale-socks

Reach your tailnet from any local app, without installing Tailscale on the machine.

`tailscale-socks` joins your tailnet as a **userspace** node — WireGuard runs in-process, so there is no `tailscaled`, no root and no TUN device — and opens three local front doors onto it:

| Front door | Default | What it gives you |
|---|---|---|
| SOCKS5 | `127.0.0.1:1080` | TCP into the tailnet, names resolved on the tailnet side |
| HTTP proxy | `127.0.0.1:8080` | `CONNECT` tunnels and plain proxied requests |
| DNS | `127.0.0.1:5354` | MagicDNS, split DNS and exit-node DNS, over UDP and TCP |

Outbound traffic can leave through an **exit node**, and **subnet routers** are reachable by default. This node only consumes the tailnet: no route or exit-node advertising, no Taildrop, no `serve`/`funnel`, no Tailscale SSH.

## 1. Install

One command, no administrator. It unpacks the release into your home — `~/.local/bin` and `~/.local/share`, or `~/bin` on Windows:

```sh
curl -fsSL https://raw.githubusercontent.com/d0whc3r/tailscale-socks/main/contrib/install.sh | sh
```

On Windows that needs a zsh — MSYS2, Cygwin or Git Bash — which the shell helpers need anyway. The release itself is one archive per platform, and nothing else:

| Platform | Archive |
|---|---|
| macOS | `tailscale-socks-darwin-universal.tar.gz` |
| Linux | `tailscale-socks-linux-amd64.tar.gz` |
| Windows | `tailscale-socks-windows-amd64.zip` |

Linux and Windows are x86-64; the macOS binary is universal and runs on Apple silicon and Intel alike. On any other architecture, build from source: see [CONTRIBUTING.md](CONTRIBUTING.md). Every archive is listed in `SHA256SUMS.txt`:

```sh
sha256sum -c --ignore-missing SHA256SUMS.txt   # shasum -a 256 -c on macOS
```

There is no `.pkg`, `.deb` or setup `.exe`, for one reason: nothing here is signed with a Developer ID, and on macOS a browser download is quarantined — Gatekeeper then kills the binary on sight, with `Killed: 9` and no explanation. `curl` sets no quarantine attribute, so what it writes just runs.

Later, update in place:

```sh
tailscale-socks upgrade
```

## 2. Load the shell helpers

Add this to `~/.zshrc`, the same line on all three platforms:

```sh
source "$HOME/.local/share/tailscale-socks/contrib/tailscale-socks.zsh"
```
 The installer already wrote `~/.tailscale/.env` for you, with every line commented out, so an untouched copy keeps the defaults — edit it when you want to change one.

`tailscale-socks upgrade` refreshes that helper along with the binary, and writes the release's template to `~/.tailscale/.env.example`. Your `~/.tailscale/.env` is never touched: it is yours, and it may hold `TS_AUTHKEY`.

## 3. Start it

```sh
tailscale-socks
```

The first run prints a login URL. Approve it in the browser and the node appears in your tailnet as `ts-proxy`; the login is saved, so later runs start straight away. For an unattended machine set `TS_AUTHKEY=tskey-auth-...` instead.

To keep it running in the background — a launchd user agent on macOS, a systemd user unit on Linux, a Task Scheduler task on Windows:

```sh
ts_install          # write the service, enable autostart, start it
ts_status           # service state, listener probes, last node summary
ts_proxy on         # send this shell's commands through the proxies
```

`ts_up`, `ts_down`, `ts_restart`, `ts_logs` and `ts_uninstall` are in [docs/service.md](docs/service.md). When something does not work, [docs/troubleshooting.md](docs/troubleshooting.md).

## 4. Use it

```sh
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
ALL_PROXY=socks5h://127.0.0.1:1080 some-app
```

Use `socks5h://` (curl: `--socks5-hostname`), **not** `socks5://`. Names must be resolved by the proxy, on the tailnet, not by your host — that is what makes MagicDNS names, split-DNS domains and an exit node's own resolvers work. Public names fall back to normal resolution. [docs/proxies.md](docs/proxies.md) goes through all three front doors.

## Commands

```sh
tailscale-socks                 # run the proxies (the default command)
tailscale-socks status          # join, print what this node can reach, exit
tailscale-socks config          # print the resolved settings; no tailnet, no login
tailscale-socks upgrade         # replace the binary and the helpers with the latest release
tailscale-socks --help          # also: run --help
```

## Parameters

Settings come from the command line first, then the environment, then `.env` next to the binary, then `~/.tailscale/.env`. Every flag has a one-letter alias and an environment variable. [docs/flags.md](docs/flags.md) explains each one.

| Flag | Alias | Env | Default | Meaning |
|---|---|---|---|---|
| `--hostname` | `-n` | `TSPROXY_HOSTNAME` | `ts-proxy` | node name on the tailnet |
| `--auth-key` | `-k` | `TS_AUTHKEY` | none | auth key for unattended login |
| `--state-dir` | `-D` | `TSPROXY_STATE_DIR` | per-user config dir | where the login is persisted |
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

Every listener is optional; an empty address disables it, and all three empty is an error:

```sh
tailscale-socks -e auto -s 127.0.0.1:1080 -p '' -d ''    # SOCKS5 only
```

Aliases take their value after a space (`-s 127.0.0.1:1080`); the `=` form is a long-flag thing, so use `--socks5=` to pass an empty value.

`--exit-node` accepts `auto`, `auto:any`, `auto:geo:us`, a peer's MagicDNS base name, a Tailscale IP, or `off`. `tailscale-socks status` lists the peers advertising one — [docs/routing.md](docs/routing.md) has the rest.

Two more variables are read by Tailscale itself rather than by a flag: `TS_CONTROL_URL` points at a self-hosted control server (Headscale), and `TSNET_FORCE_LOGIN=1` makes an auth key apply to an already logged-in node.

## Security

The proxies and the DNS server have **no authentication**. They bind to `127.0.0.1` by default; binding them to `0.0.0.0` turns the machine into an open relay into your tailnet for anyone who can reach the port.

The auth key and the state file are credentials: keep `.env` files at `0600`, and treat the state directory as a secret. Neither is ever printed.

Port 5354 is the DNS default because 53 needs root and 5353 collides with mDNS.

Found a vulnerability? Report it privately — see [SECURITY.md](SECURITY.md).

## Documentation

[docs/](docs/README.md) is the index. Straight to a page:

- [flags.md](docs/flags.md) — every parameter, explained
- [configuration.md](docs/configuration.md) — precedence, `.env` files, `tailscale-socks config`
- [proxies.md](docs/proxies.md) — the three front doors, and how a name becomes a connection
- [routing.md](docs/routing.md) — exit nodes, subnet routers, tailnet DNS
- [state.md](docs/state.md) — the login, the state directory, auth keys
- [service.md](docs/service.md) — running it in the background
- [troubleshooting.md](docs/troubleshooting.md) — what usually goes wrong
- [architecture.md](docs/architecture.md) — how the program is put together
- [CONTRIBUTING.md](CONTRIBUTING.md) — building, testing and changing it
- [SECURITY.md](SECURITY.md) — threat model and how to report a vulnerability
- [.env.example](.env.example) — every variable, with its default

## License

[MIT](LICENSE) © d0whc3r
