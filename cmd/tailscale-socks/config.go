package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/josep/tailscale-socks/internal/tsnode"
)

// configCmd prints the configuration the other commands would run with, once
// the command line, the environment and the .env files have been merged. It
// exists so a shell can ask for a resolved value instead of reimplementing
// that precedence over the same files.
type configCmd struct {
	Key string `arg:"" optional:"" help:"Print only this setting, unquoted. Takes the flag name or its environment variable."`

	listenFlags `embed:""`
	nodeFlags   `embed:""`
}

// setting is one resolved value, under both names it answers to.
type setting struct {
	flag, env, value string
}

func (c *configCmd) Run() error { return c.write(os.Stdout) }

func (c *configCmd) write(w io.Writer) error {
	settings, err := c.settings()
	if err != nil {
		return err
	}
	if c.Key == "" {
		for _, s := range settings {
			fmt.Fprintf(w, "%s=%s\n", s.env, shellQuote(s.value))
		}
		return nil
	}

	key := normalizeKey(c.Key)
	names := make([]string, len(settings))
	for i, s := range settings {
		if s.flag == key {
			fmt.Fprintln(w, s.value)
			return nil
		}
		names[i] = s.flag
	}
	return fmt.Errorf("unknown setting %q; one of: %s", c.Key, strings.Join(names, ", "))
}

func (c *configCmd) settings() ([]setting, error) {
	stateDir := c.StateDir
	if stateDir == "" {
		var err error
		if stateDir, err = tsnode.DefaultStateDir(c.Hostname); err != nil {
			return nil, err
		}
	}
	// An empty exit node and "off" mean the same thing to the node; print the
	// spelling the documentation uses.
	exitNode := c.ExitNode
	if exitNode == "" {
		exitNode = "off"
	}
	// The auth key is missing on purpose: it is a credential, and this output
	// is meant to be printed, piped and eval'd.
	return []setting{
		{"hostname", "TSPROXY_HOSTNAME", c.Hostname},
		{"state-dir", "TSPROXY_STATE_DIR", stateDir},
		{"socks5", "TSPROXY_SOCKS5", c.Socks5},
		{"http", "TSPROXY_HTTP", c.HTTP},
		{"dns", "TSPROXY_DNS", c.DNS},
		{"exit-node", "TSPROXY_EXIT_NODE", exitNode},
		{"exit-node-allow-lan", "TSPROXY_EXIT_NODE_ALLOW_LAN", strconv.FormatBool(c.ExitNodeAllowLan)},
		{"accept-routes", "TSPROXY_ACCEPT_ROUTES", strconv.FormatBool(c.AcceptRoutes)},
		{"accept-dns", "TSPROXY_ACCEPT_DNS", strconv.FormatBool(c.AcceptDns)},
		{"verbose", "TSPROXY_VERBOSE", strconv.FormatBool(c.Verbose)},
	}, nil
}

// normalizeKey turns either name a setting answers to — the flag or its
// environment variable — into the flag name.
func normalizeKey(k string) string {
	k = strings.ToLower(strings.ReplaceAll(k, "_", "-"))
	return strings.TrimPrefix(k, "tsproxy-")
}

// shellQuote makes a value safe to eval, whatever a state directory happens to
// be called.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
