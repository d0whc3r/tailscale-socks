# Documentation

The [README](../README.md) is the whole getting-started path: install, load the
shell helpers, run it. Everything here is the detail that did not belong there.

| Page | What is in it |
|---|---|
| [flags.md](flags.md) | every parameter, what it does and when to change it |
| [configuration.md](configuration.md) | where settings come from, `.env` files, `tailscale-socks config` |
| [proxies.md](proxies.md) | the three front doors, and how a name becomes a connection |
| [routing.md](routing.md) | exit nodes, subnet routers, tailnet DNS |
| [state.md](state.md) | the login, the state directory, auth keys |
| [service.md](service.md) | running it in the background |
| [troubleshooting.md](troubleshooting.md) | what usually goes wrong, and the message it prints |
| [architecture.md](architecture.md) | how the program is put together — for contributors |

Also worth reading: [.env.example](../.env.example) documents every variable
with its default, and [SECURITY.md](../SECURITY.md) has the threat model.
