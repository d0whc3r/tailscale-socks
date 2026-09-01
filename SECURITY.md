# Security policy

## Reporting a vulnerability

Report it privately through GitHub: **Security → Advisories → Report a vulnerability** on this repository. That opens a private thread with the maintainer.

Please do **not** open a public issue for a suspected vulnerability.

Useful in a report: what you ran, what you expected, what happened, and the smallest reproduction you have. Redact anything real — an auth key, the contents of `tailscaled.state`, or the tailnet names in a log are credentials or private data.

This is a single-maintainer project. Expect a first reply within a week, best effort. If a fix is warranted it lands in `main` and in the next tagged release, with an advisory published from that same private thread.

## Supported versions

The latest tagged release. There are no maintenance branches: fixes go to `main` and get a new tag.

## Threat model

`tailscale-socks` is a userspace Tailscale node that opens local front doors onto a tailnet. Whoever can reach a listener gets the node's access to the tailnet. Two consequences drive everything below:

- **The proxies and the DNS server have no authentication.** This is by design. The defaults bind `127.0.0.1`, so the boundary is "who can open a socket on this machine", not a password.
- **The state directory is a credential.** `tailscaled.state` holds the node's private keys; anyone who can read it can be this node. It is written `0600` in a `0700` directory.

### In scope

- Anything that leaks `TS_AUTHKEY` or the state file into a log line, an error message, or the `status` / `config` output.
- Anything that makes a listener reachable beyond the address it was configured with, or that changes a default bind away from loopback.
- A crash or memory-safety problem reachable from a DNS message, a SOCKS5 handshake or an HTTP request — the DNS server parses attacker-supplied bytes.
- Traffic that escapes the intended path: a request that bypasses the exit node when one is selected, or that reaches the local LAN without `--exit-node-allow-lan`.
- State-directory or `.env` files created with permissions wider than documented.
- A vulnerable dependency reachable from this code.

### Not a vulnerability

- Running with a listener bound to `0.0.0.0` or a LAN address. That is a configuration choice, and it makes the machine an open relay into your tailnet — the README says so.
- Another local user or process on the same machine using the proxies. There is no authentication; loopback is the boundary.
- A `.env` file you left world-readable. The program warns about it at startup.
- Anything Tailscale itself is responsible for — ACLs, key expiry, control plane. Report those to [Tailscale](https://tailscale.com/security-policies).

## Dependencies

`make vuln` runs `govulncheck` over the module tree. CI runs it on every push and pull request, and weekly on a schedule so advisories published after the last commit are still caught. Dependabot opens grouped PRs for direct Go dependencies and GitHub Actions.
