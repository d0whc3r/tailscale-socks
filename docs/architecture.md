# Architecture

About 1300 lines of Go in three packages. No server, no UI, no database — one
process that joins a tailnet and forwards local connections into it.

```
cmd/tailscale-socks   CLI: flags, help, .env loading, wiring, error reporting
internal/tsnode       the Tailscale node: prefs, exit node, DNS, dialing
internal/proxy        SOCKS5, HTTP and DNS servers over a tailnet dialer
contrib/              zsh helpers, launchd agent and systemd unit
.env.example          every variable, with its default
Makefile              build, check, cover, vuln, outdated, release
```

## The shape

```
   client                 tailscale-socks                    tailnet
                    ┌───────────────────────────┐
curl --socks5 ─────▶│ proxy.ServeSOCKS5         │
curl --proxy  ─────▶│ proxy.NewHTTPProxy        │──▶ DialFunc  ──▶ WireGuard
dig -p 5354   ─────▶│ proxy.ServeDNS{UDP,TCP}   │──▶ DNSBackend ──▶ (in-process)
                    └───────────────────────────┘
                              cmd/ wires the two halves together
```

`internal/proxy` speaks the wire protocols and knows nothing about Tailscale.
`internal/tsnode` owns the tailnet and knows nothing about SOCKS5 or HTTP.
`cmd/` is the only place that sees both.

**`internal/proxy` must never import `internal/tsnode`.** The two seams between
them are declared by the consumer, in `proxy`:

```go
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type DNSBackend interface {
    DNSQuery(ctx context.Context, query []byte, family string, from netip.AddrPort) ([]byte, error)
    HandleDNSTCPConn(conn net.Conn, src netip.AddrPort)
}
```

`*tsnode.Node` satisfies both without knowing they exist. That is what makes
every server testable with a fake dialer and no tailnet — see
[CONTRIBUTING.md](../CONTRIBUTING.md#tests).

## The three front doors

**SOCKS5** (`internal/proxy/socks5.go`) is `things-go/go-socks5` with two
options: `WithDial(node.DialContext)` and a resolver that returns a nil IP. The
nil IP is deliberate — it makes go-socks5 hand the host name to the dialer
verbatim, so the name is resolved on the tailnet side. Clients must therefore
use `socks5h://`.

**HTTP** (`internal/proxy/http.go`) is a `net/http` handler, not a proxy
library. `CONNECT` gets a hijacked socket splice; an absolute-URI request is
cloned, stripped of hop-by-hop headers and sent through a shared
`http.Transport` whose `DialContext` is the tailnet dialer, so connections are
pooled.

**DNS** (`internal/proxy/dns.go`) reads UDP datagrams and length-prefixed TCP
messages and hands the raw bytes to the backend. It parses nothing: Tailscale's
own `dns.Manager` answers the query with the tailnet's DNS configuration —
MagicDNS, split DNS, and the exit node's resolvers when one is in use.

## The node

`tsnode.Start` brings up a `tsnet.Server`, waits for it to be `Running`, then
applies preferences in one `EditPrefs` call: `RouteAll` (subnet routes),
`CorpDNS` (tailnet DNS), `ExitNodeAllowLANAccess`, and the exit-node selection
from `exitnode.go`.

`Node.DialContext` is the single point every server dials through, so name
resolution is fixed in one place:

1. `addr` is already an IP → dial it.
2. Otherwise resolve the name through the tailnet DNS (`LookupIP`, an A and an
   AAAA query built with `dnsmessage`), and try each address in turn.
3. Resolution failed or returned nothing → fall back to tsnet's own dialer,
   which knows netmap names and then the host resolver. A public name still
   works.

## Decisions worth knowing

**The state directory keys on the hostname, not the binary.** tsnet's own
default is `tsnet-<executable name>`, so renaming or moving the binary would
silently lose the login and register a second node. `DefaultStateDir` returns
`<user config dir>/tailscale-socks/<hostname>` instead.

**Listeners are bound before joining the tailnet.** A typo, a busy port or an
address this machine does not own then costs nothing instead of a login. The
sockets just queue connections until the servers start.

**Each server reports its error on one buffered channel**, sized for every
goroutine that can send, so none of them blocks after the first error is read.
`main` selects between that channel and a `signal.NotifyContext`.

**`tsnet.Server.Close` is not safe after a failed `Up`.** It dereferences
subsystems that only exist once the start sequence got far enough, so closing
in that window panics with a nil pointer and buries the real error.
`closeStarted` checks `ts.Sys() != nil` first.

**Errors are one line plus a hint.** `cmd/errors.go` turns a `*net.OpError` on
`listen` into "another process is already listening on …", and a permission
error into a pointer at `--state-dir`. No stack traces: the reader is meant to
fix the problem, not debug this program.

**Credentials never reach a log line.** `TS_AUTHKEY` and the state file are
secrets. `config` prints everything else — it is made to be piped and `eval`'d.

**A flag is a four-place contract.** The kong struct tag, `.env.example`, the
README table and `configCmd.settings` all describe the same flag;
`TestFlagEnvVars` fails when the environment variable drifts.

## Dependencies

Five direct, all load-bearing:

| Module | Why |
|---|---|
| [`tailscale.com`](https://tailscale.com) | `tsnet` (the node), `ipn` (prefs), `dns.Manager` (resolution) |
| [`alecthomas/kong`](https://github.com/alecthomas/kong) | CLI: commands, flags, environment variables, help |
| [`things-go/go-socks5`](https://github.com/things-go/go-socks5) | the SOCKS5 protocol |
| [`joho/godotenv`](https://github.com/joho/godotenv) | reading `.env` files |
| `golang.org/x/net` | `dnsmessage`, to build and parse the two lookup queries |

Everything else — the HTTP proxy, address handling, the listeners — is the
standard library.
