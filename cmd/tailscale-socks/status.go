package main

import (
	"context"
	"fmt"
	"log"

	"github.com/d0whc3r/tailscale-socks/internal/tsnode"
)

// statusCmd reports; it does not configure. It embeds nodeFlags and not
// prefFlags on purpose: the preferences EditPrefs writes are persisted, so a
// status that took --exit-node would leave the tailnet changed behind it, and
// one that took none would still clear what run had set.
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
