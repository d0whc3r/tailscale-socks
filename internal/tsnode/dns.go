package tsnode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"
)

// ErrNoDNS is returned when the node has no usable DNS subsystem, which
// happens when tailnet DNS was disabled with AcceptDNS=false.
var ErrNoDNS = errors.New("tailnet DNS is not available (accept-dns disabled?)")

// HasDNS reports whether DNS queries can be served.
func (n *Node) HasDNS() bool { return n.dns != nil }

// DNSQuery answers a wire-format DNS query using the tailnet DNS
// configuration: MagicDNS names, split-DNS domains and, when an exit node is
// in use, that node's resolvers. family is "udp" or "tcp".
func (n *Node) DNSQuery(ctx context.Context, query []byte, family string, from netip.AddrPort) ([]byte, error) {
	if n.dns == nil {
		return nil, ErrNoDNS
	}
	return n.dns.Query(ctx, query, family, from)
}

// HandleDNSTCPConn serves DNS over an accepted TCP connection and closes it.
func (n *Node) HandleDNSTCPConn(conn net.Conn, src netip.AddrPort) {
	if n.dns == nil {
		conn.Close()
		return
	}
	n.dns.HandleTCPConn(conn, src)
}

// LookupIP resolves host through the tailnet DNS configuration.
func (n *Node) LookupIP(ctx context.Context, host string) ([]netip.Addr, error) {
	if n.dns == nil {
		return nil, ErrNoDNS
	}
	name, err := dnsmessage.NewName(dnsname(host))
	if err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	var errs []error
	for _, typ := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		got, err := n.lookup(ctx, name, typ)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		addrs = append(addrs, got...)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %q: %w", host, errors.Join(errs...))
	}
	return addrs, nil
}

func (n *Node) lookup(ctx context.Context, name dnsmessage.Name, typ dnsmessage.Type) ([]netip.Addr, error) {
	// The query never leaves this process, so a fixed ID is fine.
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: typ, Class: dnsmessage.ClassINET}},
	}
	packed, err := msg.Pack()
	if err != nil {
		return nil, err
	}
	resp, err := n.DNSQuery(ctx, packed, "udp", netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 0))
	if err != nil {
		return nil, err
	}
	return parseAddrs(resp)
}

func parseAddrs(resp []byte) ([]netip.Addr, error) {
	var p dnsmessage.Parser
	h, err := p.Start(resp)
	if err != nil {
		return nil, err
	}
	if h.RCode != dnsmessage.RCodeSuccess {
		return nil, fmt.Errorf("dns: %s", h.RCode)
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	for {
		hdr, err := p.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			return addrs, nil
		}
		if err != nil {
			return nil, err
		}
		switch hdr.Type {
		case dnsmessage.TypeA:
			r, err := p.AResource()
			if err != nil {
				return nil, err
			}
			addrs = append(addrs, netip.AddrFrom4(r.A))
		case dnsmessage.TypeAAAA:
			r, err := p.AAAAResource()
			if err != nil {
				return nil, err
			}
			addrs = append(addrs, netip.AddrFrom16(r.AAAA))
		default:
			if err := p.SkipAnswer(); err != nil {
				return nil, err
			}
		}
	}
}

// dnsname returns host as a fully qualified name.
func dnsname(host string) string {
	if len(host) > 0 && host[len(host)-1] == '.' {
		return host
	}
	return host + "."
}
