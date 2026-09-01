// Package proxy serves SOCKS5, HTTP and DNS on local listeners, forwarding
// everything through a caller-supplied tailnet dialer and resolver.
package proxy

import (
	"context"
	"net"
	"net/netip"
)

// DialFunc opens a connection through the tailnet.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DNSBackend answers DNS queries with the tailnet DNS configuration.
// *tsnode.Node implements it.
type DNSBackend interface {
	DNSQuery(ctx context.Context, query []byte, family string, from netip.AddrPort) ([]byte, error)
	HandleDNSTCPConn(conn net.Conn, src netip.AddrPort)
}
