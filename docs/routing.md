# Exit nodes, subnet routers and DNS

What the node accepts from the tailnet, and where outbound traffic leaves.

## Exit nodes

An exit node is a peer that will forward your traffic to the public internet.
Without one, only tailnet addresses and whatever the host can already reach go
anywhere.

```sh
tailscale-socks --exit-node auto        # best one, re-picked automatically
tailscale-socks --exit-node auto:any    # the same, written as an expression
tailscale-socks --exit-node auto:geo:us # restrict the automatic pick
tailscale-socks --exit-node gateway     # a specific peer, by MagicDNS base name
tailscale-socks --exit-node 100.64.0.2  # or by Tailscale IP
tailscale-socks --exit-node off         # none (the default)
```

`off`, `none` and an empty value all mean the same thing. A peer name or IP that
matches nothing is an error, and it is reported before anything is served.

`tailscale-socks status` lists every peer advertising an exit node, and which
one is in use:

```
exit node: gateway.tailnet.ts.net online=true
exit node candidates:
  - gateway.tailnet.ts.net (online=true)
```

With `auto` and no pick made yet it says so instead:

```
exit node: auto:any (none selected yet)
```

### The local LAN

While an exit node is in use, traffic to the machine's own LAN goes through the
exit node too. `--exit-node-allow-lan` keeps it local, which is what you want if
the same machine also talks to a printer or a NAS on its own network.

## Subnet routers

A subnet router advertises a private range to the tailnet. `--accept-routes` is
on by default, so those ranges are reachable through the proxies without any
extra configuration. `status` lists them:

```
subnet routers:
  - office-router.tailnet.ts.net -> 192.168.10.0/24
```

Turn it off with `--no-accept-routes` when a route on the tailnet would collide
with a range this machine already uses.

## Tailnet DNS

`--accept-dns` is on by default and pulls in the tailnet's DNS configuration:

- **MagicDNS** — `peer` and `peer.tailnet.ts.net` resolve to tailnet addresses.
- **Split DNS** — the domains your tailnet routes to specific resolvers.
- **Exit-node DNS** — with an exit node in use, its resolvers answer, so names
  resolve from where the traffic actually leaves.

Everything that resolves a name goes through it: the `--dns` server, and the
lookup the SOCKS5 and HTTP proxies do before dialing. That is why `--dns`
without it is rejected up front, and why turning it off leaves you with IPs and
whatever the host resolver already knew.

Names the tailnet cannot answer fall back to normal resolution — see
[proxies.md](proxies.md#how-a-name-becomes-a-connection).
