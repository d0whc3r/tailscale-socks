// Package tsnode runs a userspace Tailscale node (via tsnet) and exposes the
// tailnet features the proxies need: dialing, DNS resolution, exit nodes and
// subnet routes.
package tsnode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/dns"
	"tailscale.com/tsnet"
	"tailscale.com/types/logger"
)

// Config describes the node to bring up. The zero value is usable except for
// Hostname, which control uses to name the node.
type Config struct {
	Hostname string // node name on the tailnet
	StateDir string // where to persist node state; "" means DefaultStateDir
	AuthKey  string // optional auth key for unattended login

	// ExitNode is "", "off", "auto", "auto:<expr>", a peer name or a
	// Tailscale IP. See setExitNode.
	ExitNode string
	// ExitNodeAllowLAN keeps local subnets reachable while an exit node is in
	// use.
	ExitNodeAllowLAN bool

	// AcceptRoutes accepts subnet routes advertised by subnet routers.
	AcceptRoutes bool
	// AcceptDNS uses the tailnet DNS configuration (MagicDNS, split DNS,
	// exit-node DNS). Required for the DNS server and for resolving names
	// through the tailnet.
	AcceptDNS bool

	// Logf receives user-facing messages (login URLs, state changes).
	Logf logger.Logf
	// DebugLogf receives tsnet's verbose internal logs. nil discards them.
	DebugLogf logger.Logf
}

// Node is a running userspace Tailscale node.
type Node struct {
	ts       *tsnet.Server
	lc       *local.Client
	dns      *dns.Manager // nil if the DNS subsystem is unavailable
	stateDir string

	// dialTS and lookupIP are the two seams DialContext routes through, set
	// by Start to tsnet's own dialer and this node's resolver. Neither needs
	// to be a field for the program to work; they are what lets a test drive
	// the routing below without a tailnet.
	dialTS   func(ctx context.Context, network, addr string) (net.Conn, error)
	lookupIP func(ctx context.Context, host string) ([]netip.Addr, error)
}

// DefaultStateDir returns the directory holding the login for a given node
// name: <user config dir>/tailscale-socks/<hostname>.
//
// tsnet's own default is derived from the executable's file name, so renaming
// or moving the binary would silently lose the login and register a second
// node. This default only depends on the hostname.
func DefaultStateDir(hostname string) (string, error) {
	if hostname == "" {
		return "", errors.New("hostname is required")
	}
	if strings.ContainsAny(hostname, `/\`) || hostname == "." || hostname == ".." {
		return "", fmt.Errorf("invalid hostname %q", hostname)
	}
	conf, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(conf, "tailscale-socks", hostname), nil
}

// Start brings the node up on the tailnet and applies cfg's preferences.
// The caller must Close the returned Node.
func Start(ctx context.Context, cfg Config) (*Node, error) {
	logf := cfg.Logf
	if logf == nil {
		logf = logger.Discard
	}
	debugf := cfg.DebugLogf
	if debugf == nil {
		debugf = logger.Discard
	}

	stateDir := cfg.StateDir
	if stateDir == "" {
		var err error
		if stateDir, err = DefaultStateDir(cfg.Hostname); err != nil {
			return nil, err
		}
	}

	ts := &tsnet.Server{
		Hostname: cfg.Hostname,
		Dir:      stateDir,
		AuthKey:  cfg.AuthKey,
		UserLogf: logf,
		Logf:     debugf,
	}

	st, err := ts.Up(ctx)
	if err != nil {
		closeStarted(ts)
		return nil, fmt.Errorf("joining tailnet: %w", err)
	}
	lc, err := ts.LocalClient()
	if err != nil {
		closeStarted(ts)
		return nil, err
	}

	n := &Node{ts: ts, lc: lc, stateDir: stateDir}
	// LookupIP reads n.dns at call time, so taking it here is safe.
	n.dialTS, n.lookupIP = ts.Dial, n.LookupIP
	if mgr, ok := ts.Sys().DNSManager.GetOK(); ok {
		n.dns = mgr
	}
	if err := n.applyPrefs(ctx, cfg, st); err != nil {
		closeStarted(ts)
		return nil, err
	}
	return n, nil
}

// closeStarted closes ts, unless tsnet gave up before building its
// subsystems. tsnet.Server.Close dereferences them unconditionally, so closing
// after an early start failure crashes with a nil pointer and buries the real
// error. Sys is nil exactly in that window.
func closeStarted(ts *tsnet.Server) {
	if ts.Sys() != nil {
		ts.Close()
	}
}

// prefsFor maps cfg onto the preferences to send to the node. Every field this
// program owns is masked, or EditPrefs would keep the value from the last run
// and the flag would do nothing. It is split from applyPrefs so the mapping can
// be tested against a hand-built status, without a tailnet.
func prefsFor(cfg Config, st *ipnstate.Status) (*ipn.MaskedPrefs, error) {
	mp := &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			RouteAll:               cfg.AcceptRoutes,
			CorpDNS:                cfg.AcceptDNS,
			ExitNodeAllowLANAccess: cfg.ExitNodeAllowLAN,
		},
		RouteAllSet:               true,
		CorpDNSSet:                true,
		ExitNodeAllowLANAccessSet: true,
	}
	if err := setExitNode(mp, cfg.ExitNode, st); err != nil {
		return nil, err
	}
	return mp, nil
}

func (n *Node) applyPrefs(ctx context.Context, cfg Config, st *ipnstate.Status) error {
	mp, err := prefsFor(cfg, st)
	if err != nil {
		return err
	}
	if _, err := n.lc.EditPrefs(ctx, mp); err != nil {
		return fmt.Errorf("applying preferences: %w", err)
	}
	return nil
}

// Close disconnects the node from the tailnet.
func (n *Node) Close() error { return n.ts.Close() }

// DialContext connects to addr over the tailnet. Host names are resolved with
// the tailnet DNS configuration (MagicDNS and split DNS included) so that
// clients can hand the proxy a name instead of an IP.
func (n *Node) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return n.dialTS(ctx, network, addr)
	}
	ips, err := n.lookupIP(ctx, host)
	if err != nil || len(ips) == 0 {
		// Fall back to tsnet's own resolution (netmap names, then the host
		// resolver) rather than failing outright.
		return n.dialTS(ctx, network, addr)
	}
	var errs []error
	for _, ip := range ips {
		conn, err := n.dialTS(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("dial %s: %w", addr, errors.Join(errs...))
}
