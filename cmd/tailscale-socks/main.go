// Command tailscale-socks exposes a tailnet through local SOCKS5, HTTP and
// DNS proxies, backed by a userspace Tailscale node.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/alecthomas/kong"
)

type cli struct {
	Run     runCmd           `cmd:"" default:"withargs" help:"Run the proxies (default command)."`
	Status  statusCmd        `cmd:"" help:"Join the tailnet and print what this node can reach."`
	Config  configCmd        `cmd:"" help:"Print the resolved configuration, without joining the tailnet."`
	Upgrade upgradeCmd       `cmd:"" help:"Replace this binary and the helper files with the latest release."`
	Version kong.VersionFlag `short:"V" env:"-" help:"Print the version and exit."`
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
	if err == nil || errors.Is(err, context.Canceled) {
		// A cancelled context means we were interrupted before we were up,
		// which is not a failure.
		return
	}
	report(os.Stderr, err)
	os.Exit(1)
}
