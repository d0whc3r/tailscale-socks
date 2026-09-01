package main

import (
	"context"
	"fmt"
	"log"

	"github.com/d0whc3r/tailscale-socks/internal/tsnode"
)

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
