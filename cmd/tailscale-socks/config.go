package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/d0whc3r/tailscale-socks/internal/tsnode"
)

// configCmd prints the configuration run would come up with, once the
// environment and the .env files have been merged. It exists so a shell can
// ask for a resolved value instead of reimplementing that precedence over the
// same files.
//
// It takes no flag of its own: it reports the configuration, it is not a way
// to write one.
type configCmd struct {
	Key string `arg:"" optional:"" help:"Print only this setting, unquoted. Takes the flag name or its environment variable."`
}

// setting is one resolved value, under both names it answers to.
type setting struct {
	flag, env, value string
}

func (c *configCmd) Run() error {
	run, err := resolvedRun()
	if err != nil {
		return err
	}
	return c.write(os.Stdout, run)
}

// resolvedRun is the run command with an empty command line: kong fills it
// from the flag defaults and the environment, which the .env files were loaded
// into at startup. That is the whole input, since config takes no flags to
// merge on top.
func resolvedRun() (runCmd, error) {
	var run runCmd
	parser, err := kong.New(&run, kong.DefaultEnvars("TSPROXY"))
	if err != nil {
		return run, err
	}
	_, err = parser.Parse(nil)
	return run, err
}

func (c *configCmd) write(w io.Writer, run runCmd) error {
	settings, err := settingsFor(run)
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

func settingsFor(run runCmd) ([]setting, error) {
	stateDir := run.StateDir
	if stateDir == "" {
		var err error
		if stateDir, err = tsnode.DefaultStateDir(run.Hostname); err != nil {
			return nil, err
		}
	}
	// An empty exit node and "off" mean the same thing to the node; print the
	// spelling the documentation uses.
	exitNode := run.ExitNode
	if exitNode == "" {
		exitNode = "off"
	}
	// The auth key is missing on purpose: it is a credential, and this output
	// is meant to be printed, piped and eval'd.
	return []setting{
		{"hostname", "TSPROXY_HOSTNAME", run.Hostname},
		{"state-dir", "TSPROXY_STATE_DIR", stateDir},
		{"socks5", "TSPROXY_SOCKS5", run.Socks5},
		{"http", "TSPROXY_HTTP", run.HTTP},
		{"dns", "TSPROXY_DNS", run.DNS},
		{"exit-node", "TSPROXY_EXIT_NODE", exitNode},
		{"exit-node-allow-lan", "TSPROXY_EXIT_NODE_ALLOW_LAN", strconv.FormatBool(run.ExitNodeAllowLan)},
		{"accept-routes", "TSPROXY_ACCEPT_ROUTES", strconv.FormatBool(run.AcceptRoutes)},
		{"accept-dns", "TSPROXY_ACCEPT_DNS", strconv.FormatBool(run.AcceptDns)},
		{"verbose", "TSPROXY_VERBOSE", strconv.FormatBool(run.Verbose)},
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
