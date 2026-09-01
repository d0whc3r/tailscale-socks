# The login and where it lives

## The state directory

The login is stored in `tailscaled.state` (file `0600`, directory `0700`)
under:

```
<user config dir>/tailscale-socks/<hostname>
# macOS:   ~/Library/Application Support/tailscale-socks/ts-proxy
# Linux:   ~/.config/tailscale-socks/ts-proxy
# Windows: %AppData%\tailscale-socks\ts-proxy
```

Override it with `--state-dir`. `tailscale-socks status` prints the directory
in use as `state:`.

The path deliberately does **not** depend on the binary's name. `tsnet`'s own
default does, so renaming or moving the executable would silently lose the
login and register a second node; this one keys on the hostname only.

## One login per hostname

`--hostname` is the key. Two hostnames are two nodes with two state
directories and two entries in your tailnet; reusing a hostname reuses its
login.

So changing `--hostname` does not rename the node — it registers a new one. The
old one stays in the tailnet until you remove it from the admin console.

Delete the state directory to force a fresh login. Same caveat: the old node
does not disappear on its own.

## Auth keys

Without a key, the first run prints a login URL and waits for you to approve it
in a browser. That is fine for a laptop and useless for anything unattended, so
those get `TS_AUTHKEY` instead:

```sh
TS_AUTHKEY=tskey-auth-... tailscale-socks
```

The key is only read while the node is logged out. Once state exists it is
ignored — set `TSNET_FORCE_LOGIN=1` alongside it if you really do want the key
to apply to an already logged-in node.

## Treat both as secrets

`TS_AUTHKEY` joins your tailnet and the state file holds the node's private
keys. Neither is ever printed: not in a log line, not in `status`, not in
`tailscale-socks config`.

Keep any `.env` holding a key at `0600` — the program warns on start when one
is readable by other users. Back the state directory up like a secret, or not
at all.
