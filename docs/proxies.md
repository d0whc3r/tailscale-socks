# The three front doors

`tailscale-socks` serves SOCKS5, an HTTP proxy and DNS on local addresses, and
forwards everything through one userspace Tailscale node. Each one is optional
— see [flags.md](flags.md#listeners) — and none of them authenticates anything.

## SOCKS5

```sh
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
ALL_PROXY=socks5h://127.0.0.1:1080 some-app
```

**Use `socks5h://`, not `socks5://`.** The difference is who resolves the name.
With `socks5://` the client resolves it first and sends the proxy an IP — and
the client's resolver knows nothing about MagicDNS, so the name either fails or
resolves to the wrong thing. With `socks5h://` the name reaches the proxy
intact.

The server is wired so that this works: its resolver deliberately returns no
address, which makes the SOCKS5 library hand the requested name to the dialer
verbatim, and the dialer resolves it on the tailnet.

## HTTP proxy

```sh
curl --proxy http://127.0.0.1:8080 http://peer.tailnet.ts.net/
export https_proxy=http://127.0.0.1:8080
```

It handles both halves of a forward proxy:

- **`CONNECT`** opens a tunnel and copies bytes both ways. A target without a
  port is assumed to be `:443`.
- **A plain request with an absolute URI** is re-issued over the tailnet and the
  response copied back. A request without an absolute URI gets
  `400 this is a proxy, use an absolute URI` — that is a client configured to
  talk to an origin server, not to a proxy.

Hop-by-hop headers are stripped in both directions: the fixed set
(`Connection`, `Proxy-Connection`, `Keep-Alive`, `Proxy-Authenticate`,
`Proxy-Authorization`, `Te`, `Trailer`, `Transfer-Encoding`, `Upgrade`) plus
whatever the `Connection` header itself names.

Tailnet connections are pooled across requests and dropped after 90 seconds
idle. A 10-second deadline bounds the request *header* only, so a `CONNECT`
tunnel can stay open as long as it likes.

A dial that fails comes back as `502 Bad Gateway` with the reason in the body.

## DNS

```sh
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
```

UDP and TCP are both served on the same address. UDP accepts messages up to
4096 bytes, which is what EDNS0 needs; TCP is the length-prefixed form from RFC
1035 §4.2.2. Each query has five seconds to answer, so a stuck resolver cannot
pile up work for the life of the process.

Answers come from the tailnet DNS configuration, which is why this listener
requires `--accept-dns`. See [routing.md](routing.md#tailnet-dns).

Point a resolver at it — a `resolv.conf` entry, a stub resolver, a container's
`--dns` — and MagicDNS names work for every program on the machine, not just
the ones that speak SOCKS.

## How a name becomes a connection

Both proxies dial through the same path:

1. If the target is already an IP, it is dialed straight over the tailnet.
2. Otherwise the name is resolved with the tailnet DNS configuration, `A` then
   `AAAA`, and each address is tried in turn until one connects.
3. If that lookup fails or returns nothing, the dial falls back to `tsnet`'s own
   resolution — the netmap names first, then the host resolver.

Step 3 is why a public name still works: the tailnet has no answer for it, so
it is resolved normally and reached through the exit node, if one is in use.

## No authentication

None of the three listeners asks for credentials. They bind to `127.0.0.1` by
default, and that is the only thing keeping them private: an address reachable
from elsewhere makes the machine an open relay into your tailnet for anyone who
can reach the port. See [SECURITY.md](../SECURITY.md).
