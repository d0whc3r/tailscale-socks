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
	// Tailscale IP. See ExitNodeHelp.
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
		ts.Close()
		return nil, fmt.Errorf("joining tailnet: %w", err)
	}
	lc, err := ts.LocalClient()
	if err != nil {
		ts.Close()
		return nil, err
	}

	n := &Node{ts: ts, lc: lc, stateDir: stateDir}
	if mgr, ok := ts.Sys().DNSManager.GetOK(); ok {
		n.dns = mgr
	}
	if err := n.applyPrefs(ctx, cfg, st); err != nil {
		ts.Close()
		return nil, err
	}
	return n, nil
}

func (n *Node) applyPrefs(ctx context.Context, cfg Config, st *ipnstate.Status) error {
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
		return err
	}
	if _, err := n.lc.EditPrefs(ctx, mp); err != nil {
		return fmt.Errorf("applying preferences: %w", err)
	}
	return nil
}

// StateDir is the directory where the node's login is persisted.
func (n *Node) StateDir() string { return n.stateDir }

// Close disconnects the node from the tailnet.
func (n *Node) Close() error { return n.ts.Close() }

// Status returns the current tailnet status, including peers.
func (n *Node) Status(ctx context.Context) (*ipnstate.Status, error) {
	return n.lc.Status(ctx)
}

// DialContext connects to addr over the tailnet. Host names are resolved with
// the tailnet DNS configuration (MagicDNS and split DNS included) so that
// clients can hand the proxy a name instead of an IP.
func (n *Node) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return n.ts.Dial(ctx, network, addr)
	}
	ips, err := n.LookupIP(ctx, host)
	if err != nil || len(ips) == 0 {
		// Fall back to tsnet's own resolution (netmap names, then the host
		// resolver) rather than failing outright.
		return n.ts.Dial(ctx, network, addr)
	}
	var errs []error
	for _, ip := range ips {
		conn, err := n.ts.Dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("dial %s: %w", addr, errors.Join(errs...))
}

// TailscaleIPs reports the node's own tailnet addresses.
func (n *Node) TailscaleIPs() (ip4, ip6 netip.Addr) { return n.ts.TailscaleIPs() }

// ExitNodeHelp documents the accepted -exit-node values.
const ExitNodeHelp = `off | auto | auto:<expr> | <peer-name> | <tailscale-ip>`

// Describe writes a human-readable summary of the node and what it can reach.
func (n *Node) Describe(ctx context.Context) (string, error) {
	st, err := n.Status(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	self := st.Self
	fmt.Fprintf(&b, "node:     %s (%s)\n", strings.TrimSuffix(self.DNSName, "."), st.BackendState)
	fmt.Fprintf(&b, "addrs:    %s\n", joinAddrs(self.TailscaleIPs))
	if st.CurrentTailnet != nil {
		fmt.Fprintf(&b, "tailnet:  %s (MagicDNS suffix %s)\n", st.CurrentTailnet.Name, st.CurrentTailnet.MagicDNSSuffix)
	}

	prefs, err := n.lc.GetPrefs(ctx)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "state:    %s\n", n.stateDir)
	fmt.Fprintf(&b, "dns:      accept=%t\n", prefs.CorpDNS)
	fmt.Fprintf(&b, "routes:   accept=%t\n", prefs.RouteAll)

	switch {
	case st.ExitNodeStatus != nil:
		name := peerName(st, st.ExitNodeStatus.ID)
		fmt.Fprintf(&b, "exit node: %s online=%t\n", name, st.ExitNodeStatus.Online)
	case prefs.AutoExitNode.IsSet():
		fmt.Fprintf(&b, "exit node: auto:%s (none selected yet)\n", prefs.AutoExitNode)
	default:
		b.WriteString("exit node: none\n")
	}

	var exits, routers []string
	for _, ps := range st.Peer {
		name := strings.TrimSuffix(ps.DNSName, ".")
		if ps.ExitNodeOption {
			exits = append(exits, fmt.Sprintf("%s (online=%t)", name, ps.Online))
		}
		if ps.PrimaryRoutes != nil && ps.PrimaryRoutes.Len() > 0 {
			var rs []string
			for _, r := range ps.PrimaryRoutes.All() {
				rs = append(rs, r.String())
			}
			routers = append(routers, fmt.Sprintf("%s -> %s", name, strings.Join(rs, ",")))
		}
	}
	writeList(&b, "exit node candidates", exits)
	writeList(&b, "subnet routers", routers)
	if len(st.Health) > 0 {
		writeList(&b, "health warnings", st.Health)
	}
	return b.String(), nil
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s: none\n", title)
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", it)
	}
}

func joinAddrs(addrs []netip.Addr) string {
	ss := make([]string, len(addrs))
	for i, a := range addrs {
		ss[i] = a.String()
	}
	return strings.Join(ss, ", ")
}
