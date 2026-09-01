package proxy

import (
	"context"
	"log"
	"net"
	"net/netip"
)

// maxDNSMessage is the largest UDP DNS message we accept (EDNS0 allows more
// than the classic 512 bytes).
const maxDNSMessage = 4096

// ServeDNSUDP answers DNS queries received on pc using the tailnet resolver.
func ServeDNSUDP(ctx context.Context, pc net.PacketConn, b DNSBackend, logger *log.Logger) error {
	buf := make([]byte, maxDNSMessage)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		query := make([]byte, n)
		copy(query, buf[:n])
		go func() {
			src, _ := netip.ParseAddrPort(from.String())
			resp, err := b.DNSQuery(ctx, query, "udp", src)
			if err != nil {
				logger.Printf("query from %v: %v", from, err)
				return
			}
			if _, err := pc.WriteTo(resp, from); err != nil {
				logger.Printf("reply to %v: %v", from, err)
			}
		}()
	}
}

// ServeDNSTCP answers DNS queries arriving over TCP (length-prefixed, as in
// RFC 1035 section 4.2.2).
func ServeDNSTCP(ln net.Listener, b DNSBackend) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		src, _ := netip.ParseAddrPort(conn.RemoteAddr().String())
		go b.HandleDNSTCPConn(conn, src)
	}
}
