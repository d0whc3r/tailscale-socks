package main

import "github.com/d0whc3r/tailscale-socks/internal/tsnode"

// description is the help text. It documents the flags below, so it lives
// with them: adding a front door or changing the .env precedence changes
// both.
const description = `Join a tailnet as a userspace node and expose it locally.

The node needs no tailscaled and no root: it speaks WireGuard in-process and
serves three local front doors, each of which can be disabled with an empty
address:

  --socks5   SOCKS5 proxy   (use socks5h:// so names resolve on the tailnet)
  --http     HTTP proxy     (CONNECT tunnels and plain requests)
  --dns      DNS server     (MagicDNS, split DNS, exit-node DNS)

Traffic can leave through an exit node (--exit-node auto picks the best one)
and reach subnet routers (--accept-routes, on by default).

Every flag can also be set through its environment variable (shown in the flag
help) or in a .env file. Files are read from, in decreasing priority:

  1. .env next to the executable
  2. ~/.tailscale/.env

The command line beats the environment, which beats both files.

Examples:
  # First run: prints a login URL, then serves the defaults.
  tailscale-socks

  # Unattended, everything through the nearest exit node.
  TS_AUTHKEY=tskey-auth-... tailscale-socks --exit-node auto

  # SOCKS5 only, on a custom port.
  tailscale-socks --http= --dns= --socks5 127.0.0.1:9050

  # What can this node see?
  tailscale-socks status

  # What settings are in effect? (no tailnet, no login)
  tailscale-socks config
  tailscale-socks config socks5

  # Replace this binary and the helper files with the latest release.
  tailscale-socks upgrade

  curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
  curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
  dig @127.0.0.1 -p 5354 peer.tailnet.ts.net`

// nodeFlags configure the underlying Tailscale node.
type nodeFlags struct {
	Hostname string `short:"n" default:"ts-proxy" help:"Node name to register on the tailnet."`
	AuthKey  string `short:"k" env:"TS_AUTHKEY" placeholder:"KEY" help:"Auth key for unattended login. Without it, the first run prints a login URL."`
	StateDir string `short:"D" type:"path" placeholder:"DIR" help:"Directory holding the login state (default: <user config dir>/tailscale-socks/<hostname>)."`

	ExitNode         string `short:"e" placeholder:"NODE" help:"Send outbound traffic through an exit node: auto, auto:<expr>, a peer name, a Tailscale IP, or off."`
	ExitNodeAllowLan bool   `name:"exit-node-allow-lan" short:"l" help:"Keep the local LAN reachable while an exit node is in use."`

	AcceptRoutes bool `short:"r" negatable:"" default:"true" help:"Accept subnet routes advertised by subnet routers. On by default; disable with --no-accept-routes."`
	AcceptDns    bool `name:"accept-dns" short:"a" negatable:"" default:"true" help:"Use the tailnet DNS configuration: MagicDNS, split DNS, exit-node DNS. On by default; disable with --no-accept-dns."`

	Verbose bool `short:"v" help:"Also log tsnet's internal chatter."`
}

func (f nodeFlags) config(logf func(string, ...any)) tsnode.Config {
	cfg := tsnode.Config{
		Hostname:         f.Hostname,
		StateDir:         f.StateDir,
		AuthKey:          f.AuthKey,
		ExitNode:         f.ExitNode,
		ExitNodeAllowLAN: f.ExitNodeAllowLan,
		AcceptRoutes:     f.AcceptRoutes,
		AcceptDNS:        f.AcceptDns,
		Logf:             logf,
	}
	if f.Verbose {
		cfg.DebugLogf = logf
	}
	return cfg
}

// listenFlags are the local front doors, shared by run and config.
type listenFlags struct {
	// The env tag on Socks5 is not redundant: kong.DefaultEnvars splits
	// "socks5" at the letter/digit boundary and would derive
	// TSPROXY_SOCKS_5. Removing it renames the documented variable.
	Socks5 string `name:"socks5" short:"s" env:"TSPROXY_SOCKS5" default:"127.0.0.1:1080" help:"SOCKS5 listen address; empty disables it."`
	HTTP   string `name:"http" short:"p" default:"127.0.0.1:8080" help:"HTTP proxy listen address; empty disables it."`
	DNS    string `name:"dns" short:"d" default:"127.0.0.1:5354" help:"DNS server listen address (UDP and TCP); empty disables it."`
}
