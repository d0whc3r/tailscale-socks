# Troubleshooting

Errors are one line, plus a hint when there is something to do about it.
Nothing prints a stack trace.

## It refuses to start

**`nothing to serve: --socks5, --http and --dns are all empty`**
Every listener is optional but at least one has to stay.

**`--dns needs the tailnet DNS config; drop --no-accept-dns`**
The DNS server answers from the tailnet DNS configuration, so the two settings
contradict each other. See [routing.md](routing.md#tailnet-dns).

**`another process is already listening on 127.0.0.1:1080`**
Something else has the port — often a second copy of this program, or the
background service. `ts_status` says whether the service is running.

**`not allowed to bind ...; ports below 1024 need root`**
Pick a port above 1024. This program is not meant to run as root; that is the
point of a userspace node.

**`... is not an address of this machine`**
A typo in the listen address, or an interface that does not exist here.

**`the state directory must be a writable directory; check --state-dir`**
`--state-dir` points at a file, or at somewhere this user cannot write.

All of these are checked before the node joins the tailnet, so a typo never
costs you a login.

## Names do not resolve

**A MagicDNS name works with `dig` but not through the proxy.**
The client is using `socks5://` instead of `socks5h://`, so it is resolving the
name itself. With curl that is `--socks5-hostname`. See
[proxies.md](proxies.md#socks5).

**Nothing resolves at all.**
`--no-accept-dns` turns off the tailnet DNS configuration, which is what every
lookup goes through. `tailscale-socks status` prints `dns: accept=false` when
that is the case.

**`dig` gets nothing on port 53.**
The default is 5354: port 53 needs root and 5353 collides with mDNS. Use
`dig @127.0.0.1 -p 5354`.

## The HTTP proxy answers instead of the target

**`400 this is a proxy, use an absolute URI`**
The client is talking to it as if it were a web server. Configure it as a
proxy — `http_proxy` / `--proxy`, not a base URL.

**`502 Bad Gateway`**
The tailnet dial failed; the reason is in the body. Usually the peer is offline,
the name does not exist, or the port is closed.

## The tailnet side

**The auth key seems to be ignored.**
It only applies while the node is logged out. Once the state directory holds a
login, set `TSNET_FORCE_LOGIN=1` as well. See [state.md](state.md#auth-keys).

**A second node appeared in the admin console.**
`--hostname` changed, or the state directory moved. Each hostname is its own
node with its own login.

**Public sites do not load.**
Without an exit node only the tailnet and whatever the host can already reach
are reachable. `--exit-node auto` picks one; `status` lists the candidates.

**`health warnings:` in the status output.**
That is Tailscale's own diagnosis of the node, passed through unchanged.

## Anything else

`--verbose` adds `tsnet`'s internal logs, which is where a failing join explains
itself. `tailscale-socks config` prints the settings actually in effect, without
joining anything — useful when a `.env` is not the one you think it is.

## macOS kills the binary: `Killed: 9`

Nothing here is signed with a Developer ID, so anything that arrives through a
browser is quarantined, and Gatekeeper kills a quarantined unsigned binary with
no message at all. Install with the `curl | sh` line from the README — `curl`
sets no quarantine attribute — or clear it by hand:

```sh
xattr -d com.apple.quarantine ~/.local/bin/tailscale-socks
```
