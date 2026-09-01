# Parameters

The one-line table is in the [README](../README.md#parameters). This page is
the detail: what each parameter is for, and what happens when you change it.

Every parameter can be given three ways — a flag, its one-letter alias, or an
environment variable — and the environment variable can come from a `.env`
file. [configuration.md](configuration.md) has the precedence rules.

## Syntax

Long flags take their value after a space or an `=`:

```sh
tailscale-socks --socks5 127.0.0.1:9050
tailscale-socks --socks5=127.0.0.1:9050
```

Aliases take it after a space only. The `=` form is a long-flag thing, so an
empty value has to be written `--socks5=`, not `-s=`:

```sh
tailscale-socks -s 127.0.0.1:9050
tailscale-socks --http= --dns=          # HTTP and DNS off, SOCKS5 left alone
```

`--accept-routes` and `--accept-dns` are on by default and are turned off by
their negative form, `--no-accept-routes` and `--no-accept-dns`.

Environment variables are `TSPROXY_` plus the flag name in upper case, with two
exceptions: `TS_AUTHKEY` and `TS_CONTROL_URL` are Tailscale's own names and are
never renamed or prefixed.

## Node

### `--hostname`, `-n`, `TSPROXY_HOSTNAME` — default `ts-proxy`

The name this node registers with on the tailnet. It is also the key to the
login: each hostname is a separate node with its own state directory, so
changing it registers a *second* node rather than renaming the first. The old
one stays in your tailnet until you remove it from the admin console. See
[state.md](state.md).

It becomes a directory name, so it cannot contain `/` or `\` and cannot be `.`
or `..`.

### `--auth-key`, `-k`, `TS_AUTHKEY`

An auth key logs the node in without a browser, which is what an unattended
machine needs. Without one, the first run prints a login URL and waits.

It is only used while the node is logged out. Once the state directory holds a
login the key is ignored, unless `TSNET_FORCE_LOGIN=1` is also set.

The key is a credential: it is never logged, and `tailscale-socks config` does
not print it.

### `--state-dir`, `-D`, `TSPROXY_STATE_DIR`

Where the login is persisted. The default is
`<user config dir>/tailscale-socks/<hostname>` — see [state.md](state.md) for
the per-platform paths and why the path deliberately ignores the binary's name.

## Listeners

All three are optional: an empty address disables that front door. All three
empty is an error, since there would be nothing left to serve.

Each one is bound *before* the node joins the tailnet, so a typo, a busy port
or an address this machine does not own costs nothing instead of costing a
login. [proxies.md](proxies.md) explains what each one actually does.

### `--socks5`, `-s`, `TSPROXY_SOCKS5` — default `127.0.0.1:1080`

The SOCKS5 listen address. Clients must use `socks5h://` (curl:
`--socks5-hostname`) so that names are resolved on the tailnet and not by the
client.

### `--http`, `-p`, `TSPROXY_HTTP` — default `127.0.0.1:8080`

The HTTP forward-proxy listen address: `CONNECT` tunnels and plain absolute-URI
requests.

### `--dns`, `-d`, `TSPROXY_DNS` — default `127.0.0.1:5354`

The DNS listen address. Both UDP and TCP are bound on it.

The port is 5354 because 53 needs root and 5353 collides with mDNS. This
listener needs the tailnet DNS configuration, so combining it with
`--no-accept-dns` is rejected before the node joins.

## Routing

### `--exit-node`, `-e`, `TSPROXY_EXIT_NODE` — default `off`

Send outbound traffic through an exit node. Accepted forms:

| Value | Meaning |
|---|---|
| `off`, `none`, empty | no exit node |
| `auto` | let Tailscale pick the best one and re-pick as needed |
| `auto:any` | the same, written as an expression |
| `auto:geo:us` | restrict the automatic pick |
| `gateway` | a specific peer, by MagicDNS base name |
| `100.64.0.2` | a specific peer, by Tailscale IP |

A name or IP that no peer matches is an error, reported before anything is
served. [routing.md](routing.md) has the rest.

### `--exit-node-allow-lan`, `-l`, `TSPROXY_EXIT_NODE_ALLOW_LAN` — default off

While an exit node is in use, keep the machine's own LAN reachable instead of
sending that traffic through the exit node too.

### `--accept-routes`, `-r`, `TSPROXY_ACCEPT_ROUTES` — default on

Accept the subnet routes advertised by subnet routers on the tailnet, so their
private ranges are reachable through the proxies. `tailscale-socks status`
lists the routers it can see. Turn it off with `--no-accept-routes`.

### `--accept-dns`, `-a`, `TSPROXY_ACCEPT_DNS` — default on

Use the tailnet's DNS configuration: MagicDNS names, split-DNS domains and,
when an exit node is in use, that node's own resolvers.

This is what makes a name work at all — both the `--dns` server and the name
resolution the SOCKS5 and HTTP proxies do internally go through it. Turn it off
with `--no-accept-dns` and you are left with IPs and whatever the host resolver
knows.

## Logging and information

### `--verbose`, `-v`, `TSPROXY_VERBOSE` — default off

Also log `tsnet`'s internal chatter. Useful when a join fails; noisy otherwise.

### `--version`, `-V` and `--help`, `-h`

`--version` prints the version, which comes from the release tag through
`debug.ReadBuildInfo`. `--help` prints the full help; `tailscale-socks run
--help` prints the flags for `run` alone.

## Read by Tailscale, not by a flag

| Variable | What it does |
|---|---|
| `TS_CONTROL_URL` | point at a self-hosted control server, such as Headscale |
| `TSNET_FORCE_LOGIN=1` | make `TS_AUTHKEY` apply even when the node is already logged in |
