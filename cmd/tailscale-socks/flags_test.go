package main

import (
	"testing"

	"github.com/alecthomas/kong"
)

// cliFlags returns how kong models this CLI: the flags of the run command,
// which carry every setting, and the root flags, which are kong's own.
func cliFlags(t *testing.T) (run, root []*kong.Flag) {
	t.Helper()

	var c cli
	parser, err := kong.New(&c, kong.DefaultEnvars("TSPROXY"), kong.Vars{"version": "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range parser.Model.Node.Children {
		if child.Name == "run" {
			return child.Flags, parser.Model.Node.Flags
		}
	}
	t.Fatal(`no "run" command in the model`)
	return nil, nil
}

// TestFlagEnvVars pins the environment variable of every flag. They are a
// four-place contract with .env.example, the README table and the config dump,
// and kong derives most of them from the flag name: --socks5 in particular
// would become TSPROXY_SOCKS_5 without its explicit env tag.
func TestFlagEnvVars(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"hostname":            "TSPROXY_HOSTNAME",
		"auth-key":            "TS_AUTHKEY",
		"state-dir":           "TSPROXY_STATE_DIR",
		"socks5":              "TSPROXY_SOCKS5",
		"http":                "TSPROXY_HTTP",
		"dns":                 "TSPROXY_DNS",
		"exit-node":           "TSPROXY_EXIT_NODE",
		"exit-node-allow-lan": "TSPROXY_EXIT_NODE_ALLOW_LAN",
		"accept-routes":       "TSPROXY_ACCEPT_ROUTES",
		"accept-dns":          "TSPROXY_ACCEPT_DNS",
		"verbose":             "TSPROXY_VERBOSE",
	}

	run, root := cliFlags(t)

	seen := make(map[string]bool, len(want))
	for _, f := range run {
		seen[f.Name] = true
		var got string
		if len(f.Envs) > 0 {
			got = f.Envs[0]
		}
		if len(f.Envs) > 1 {
			t.Errorf("--%s has several env vars %q, want at most one", f.Name, f.Envs)
		}
		w, known := want[f.Name]
		if !known {
			t.Errorf("--%s is not in the README table; add it there, to .env.example and here", f.Name)
			continue
		}
		if got != w {
			t.Errorf("--%s env = %q, want %q", f.Name, got, w)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("--%s is documented but no longer exists", name)
		}
	}

	// --help and --version are kong's own; neither takes a value from the
	// environment, and the README table says so.
	for _, f := range root {
		if (f.Name == "help" || f.Name == "version") && len(f.Envs) > 0 {
			t.Errorf("--%s reads %q, want no environment variable", f.Name, f.Envs)
		}
	}
}
