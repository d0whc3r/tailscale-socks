package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/d0whc3r/tailscale-socks/internal/proxy"
	"github.com/d0whc3r/tailscale-socks/internal/tsnode"
)

type runCmd struct {
	listenFlags `embed:""`
	nodeFlags   `embed:""`
}

func (c *runCmd) Run(ctx context.Context, logger *log.Logger) error {
	if c.Socks5 == "" && c.HTTP == "" && c.DNS == "" {
		return errors.New("nothing to serve: --socks5, --http and --dns are all empty")
	}
	if c.DNS != "" && !c.AcceptDns {
		return errors.New("--dns needs the tailnet DNS config; drop --no-accept-dns")
	}

	// Bind every listener before joining the tailnet: a typo, a busy port or
	// an address this machine does not own then costs nothing, instead of a
	// login. The sockets only queue connections until the servers start.
	var socks5Ln, httpLn, dnsLn net.Listener
	var dnsPC net.PacketConn
	if c.Socks5 != "" {
		ln, err := net.Listen("tcp", c.Socks5)
		if err != nil {
			return fmt.Errorf("socks5: %w", err)
		}
		defer ln.Close()
		socks5Ln = ln
	}
	if c.HTTP != "" {
		ln, err := net.Listen("tcp", c.HTTP)
		if err != nil {
			return fmt.Errorf("http: %w", err)
		}
		defer ln.Close()
		httpLn = ln
	}
	if c.DNS != "" {
		pc, err := net.ListenPacket("udp", c.DNS)
		if err != nil {
			return fmt.Errorf("dns/udp: %w", err)
		}
		defer pc.Close()
		ln, err := net.Listen("tcp", c.DNS)
		if err != nil {
			return fmt.Errorf("dns/tcp: %w", err)
		}
		defer ln.Close()
		dnsPC, dnsLn = pc, ln
	}

	node, err := tsnode.Start(ctx, c.config(logger.Printf))
	if err != nil {
		return err
	}
	defer node.Close()

	if dnsLn != nil && !node.HasDNS() {
		return tsnode.ErrNoDNS
	}
	if summary, err := node.Describe(ctx); err != nil {
		logger.Printf("describing node: %v", err)
	} else {
		fmt.Print(summary)
	}

	// Buffered for every server below, so none of them blocks on the send
	// once the first error has been read.
	errc := make(chan error, 4)
	if socks5Ln != nil {
		logger.Printf("SOCKS5 proxy on %s", socks5Ln.Addr())
		go func() { errc <- proxy.ServeSOCKS5(socks5Ln, node.DialContext, prefixed(logger, "socks5: ")) }()
	}
	if httpLn != nil {
		srv := &http.Server{
			Handler: proxy.NewHTTPProxy(node.DialContext),
			// Only bounds the request header; CONNECT tunnels stay open.
			ReadHeaderTimeout: 10 * time.Second,
		}
		logger.Printf("HTTP proxy on %s", httpLn.Addr())
		go func() { errc <- srv.Serve(httpLn) }()
	}
	if dnsLn != nil {
		logger.Printf("DNS server on %s (udp+tcp)", dnsLn.Addr())
		go func() { errc <- proxy.ServeDNSUDP(ctx, dnsPC, node, prefixed(logger, "dns: ")) }()
		go func() { errc <- proxy.ServeDNSTCP(dnsLn, node) }()
	}

	select {
	case <-ctx.Done():
		logger.Print("shutting down")
		return nil
	case err := <-errc:
		return err
	}
}

func prefixed(l *log.Logger, prefix string) *log.Logger {
	return log.New(l.Writer(), prefix, l.Flags())
}
