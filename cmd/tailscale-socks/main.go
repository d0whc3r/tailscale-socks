// Command tailscale-socks exposes a tailnet through local SOCKS5, HTTP and
// DNS proxies, backed by a userspace Tailscale node.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/josep/tailscale-socks/internal/proxy"
	"github.com/josep/tailscale-socks/internal/tsnode"
)

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

  curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
  curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
  dig @127.0.0.1 -p 5354 peer.tailnet.ts.net`

type cli struct {
	Run     runCmd           `cmd:"" default:"withargs" help:"Run the proxies (default command)."`
	Status  statusCmd        `cmd:"" help:"Join the tailnet and print what this node can reach."`
	Version kong.VersionFlag `short:"V" env:"-" help:"Print the version and exit."`
}

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

type runCmd struct {
	Socks5 string `name:"socks5" short:"s" env:"TSPROXY_SOCKS5" default:"127.0.0.1:1080" help:"SOCKS5 listen address; empty disables it."`
	HTTP   string `name:"http" short:"p" default:"127.0.0.1:8080" help:"HTTP proxy listen address; empty disables it."`
	DNS    string `name:"dns" short:"d" default:"127.0.0.1:5354" help:"DNS server listen address (UDP and TCP); empty disables it."`

	nodeFlags `embed:""`
}

func (c *runCmd) Run(ctx context.Context, logger *log.Logger) error {
	if c.Socks5 == "" && c.HTTP == "" && c.DNS == "" {
		return fmt.Errorf("nothing to serve: --socks5, --http and --dns are all empty")
	}
	if c.DNS != "" && !c.AcceptDns {
		return fmt.Errorf("--dns needs the tailnet DNS config; drop --no-accept-dns")
	}
	// Check the addresses before joining the tailnet, so a typo fails fast.
	for _, a := range []struct{ flag, addr string }{{"--socks5", c.Socks5}, {"--http", c.HTTP}, {"--dns", c.DNS}} {
		if a.addr == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(a.addr); err != nil {
			return fmt.Errorf("%s %q: %w", a.flag, a.addr, err)
		}
	}

	node, err := tsnode.Start(ctx, c.config(logger.Printf))
	if err != nil {
		return err
	}
	defer node.Close()

	if summary, err := node.Describe(ctx); err == nil {
		fmt.Print(summary)
	}

	errc := make(chan error, 3)
	if c.Socks5 != "" {
		ln, err := net.Listen("tcp", c.Socks5)
		if err != nil {
			return fmt.Errorf("socks5: %w", err)
		}
		defer ln.Close()
		logger.Printf("SOCKS5 proxy on %s", ln.Addr())
		go func() { errc <- proxy.ServeSOCKS5(ln, node.DialContext, prefixed(logger, "socks5: ")) }()
	}
	if c.HTTP != "" {
		ln, err := net.Listen("tcp", c.HTTP)
		if err != nil {
			return fmt.Errorf("http: %w", err)
		}
		defer ln.Close()
		srv := &http.Server{Handler: proxy.NewHTTPProxy(node.DialContext)}
		logger.Printf("HTTP proxy on %s", ln.Addr())
		go func() { errc <- srv.Serve(ln) }()
	}
	if c.DNS != "" {
		if !node.HasDNS() {
			return tsnode.ErrNoDNS
		}
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
		logger.Printf("DNS server on %s (udp+tcp)", ln.Addr())
		go func() { errc <- proxy.ServeDNSUDP(ctx, pc, node, prefixed(logger, "dns: ")) }()
		go func() { errc <- proxy.ServeDNSTCP(ln, node) }()
	}

	select {
	case <-ctx.Done():
		logger.Print("shutting down")
		return nil
	case err := <-errc:
		return err
	}
}

type statusCmd struct {
	nodeFlags `embed:""`
}

func (c *statusCmd) Run(ctx context.Context, logger *log.Logger) error {
	node, err := tsnode.Start(ctx, c.config(logger.Printf))
	if err != nil {
		return err
	}
	defer node.Close()

	summary, err := node.Describe(ctx)
	if err != nil {
		return err
	}
	fmt.Print(summary)
	return nil
}

func prefixed(l *log.Logger, prefix string) *log.Logger {
	return log.New(l.Writer(), prefix, l.Flags())
}

func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "devel"
}

func main() {
	var root cli
	envMessages := loadDotEnvs(dotEnvPaths())

	kctx := kong.Parse(&root,
		kong.Name("tailscale-socks"),
		kong.DefaultEnvars("TSPROXY"),
		kong.Description(description),
		kong.UsageOnError(),
		kong.Vars{"version": version()},
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stderr, "", log.LstdFlags)
	for _, m := range envMessages {
		logger.Print(m)
	}
	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(logger)

	err := kctx.Run()
	if errors.Is(err, context.Canceled) {
		// Interrupted before we were up; not a failure.
		return
	}
	kctx.FatalIfErrorf(err)
}
