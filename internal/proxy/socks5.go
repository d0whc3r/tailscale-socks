package proxy

import (
	"context"
	"log"
	"net"

	socks5 "github.com/things-go/go-socks5"
)

// ServeSOCKS5 accepts SOCKS5 connections on ln and forwards them over dial.
//
// Host names are passed through unresolved so that dial can resolve them with
// the tailnet's DNS. Clients must therefore use socks5h:// (curl:
// --socks5-hostname) for MagicDNS names to work.
func ServeSOCKS5(ln net.Listener, dial DialFunc, logger *log.Logger) error {
	srv := socks5.NewServer(
		socks5.WithDial(dial),
		socks5.WithResolver(passthroughResolver{}),
		socks5.WithLogger(socks5.NewLogger(logger)),
	)
	return srv.Serve(ln)
}

// passthroughResolver defers resolution to the dialer by returning no IP,
// which makes go-socks5 dial the requested name verbatim.
type passthroughResolver struct{}

func (passthroughResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	return ctx, nil, nil
}
