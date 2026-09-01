package tsnode

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

// Describe returns a human-readable summary of the node and what it can reach.
func (n *Node) Describe(ctx context.Context) (string, error) {
	st, err := n.lc.Status(ctx)
	if err != nil {
		return "", err
	}
	prefs, err := n.lc.GetPrefs(ctx)
	if err != nil {
		return "", err
	}
	return describe(st, prefs, n.stateDir), nil
}

// describe formats the summary. It is split from Describe so that it can be
// tested against a hand-built status, without a tailnet.
func describe(st *ipnstate.Status, prefs *ipn.Prefs, stateDir string) string {
	var b strings.Builder

	// Self is nil until control has told the node who it is.
	name, addrs := "unknown", "none"
	if self := st.Self; self != nil {
		name = strings.TrimSuffix(self.DNSName, ".")
		if len(self.TailscaleIPs) > 0 {
			addrs = joinAddrs(self.TailscaleIPs)
		}
	}
	fmt.Fprintf(&b, "node:     %s (%s)\n", name, st.BackendState)
	fmt.Fprintf(&b, "addrs:    %s\n", addrs)
	if st.CurrentTailnet != nil {
		fmt.Fprintf(&b, "tailnet:  %s (MagicDNS suffix %s)\n", st.CurrentTailnet.Name, st.CurrentTailnet.MagicDNSSuffix)
	}
	fmt.Fprintf(&b, "state:    %s\n", stateDir)
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
	return b.String()
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

// peerName returns a readable name for a node ID found in st.
func peerName(st *ipnstate.Status, id tailcfg.StableNodeID) string {
	for _, ps := range st.Peer {
		if ps.ID == id {
			return strings.TrimSuffix(ps.DNSName, ".")
		}
	}
	return string(id)
}
