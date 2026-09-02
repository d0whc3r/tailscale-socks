# Configuration reference

Where a setting comes from, and how to read back the one that won. The
parameters themselves are in [flags.md](flags.md), and the login and its state
directory in [state.md](state.md).

## Precedence

Settings come from, in decreasing priority:

1. the command line
2. the environment
3. `.env` next to the binary
4. `~/.tailscale/.env`

"Next to the binary" is the real binary's directory, symlinks resolved — so it
does not apply to `go run`, which builds into a temporary directory.

## `.env` files

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

## Keeping it up to date

`tailscale-socks upgrade` fetches the latest release from GitHub, checks it
against `SHA256SUMS.txt` and replaces three things in place: this executable,
the zsh helpers under `~/.local/share/tailscale-socks`, and the settings
template, which it writes to `~/.tailscale/.env.example`.

It never writes `~/.tailscale/.env`. That file is yours and may hold
`TS_AUTHKEY`; when a release adds a setting, the new template beside it is
where you can see it. `TSPROXY_SHARE_DIR` and `TSPROXY_ENV_DIR` move the last
two, exactly as they do for `contrib/install.sh`.

## Reading the configuration back

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
the same key. `config` takes no flag of its own: it reports what a `run` would
resolve from the environment and the `.env` files, and reporting is all it
does. An empty value means a disabled listener. The auth key is never printed:
this output is made to be piped and logged.

## The node summary

`run` and `status` print the same thing — [routing.md](routing.md) explains the
exit-node and subnet-router lines:

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
