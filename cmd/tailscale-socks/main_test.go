package main

import (
	"context"
	"io"
	"log"
	"net"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// TestRunRejectsBadFlagsBeforeJoining checks the fail-fast guards in runCmd.Run:
// every case below must be rejected before tsnode.Start is reached, so a typo or
// a busy port never costs a tailnet login. The context is already cancelled and
// the state directory is throwaway, so a regression cannot quietly join a
// tailnet here.
func TestRunRejectsBadFlagsBeforeJoining(t *testing.T) {
	// A port this process already owns: binding it again must fail.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup, not defer: the parallel subtests below run after this
	// function returns, and a deferred Close would free the port first.
	t.Cleanup(func() { busy.Close() })

	tests := []struct {
		name string
		cmd  runCmd
		want string
	}{
		{
			name: "every listener disabled",
			cmd:  runCmd{},
			want: "nothing to serve",
		},
		{
			name: "dns server without tailnet dns",
			cmd:  runCmd{listenFlags: listenFlags{DNS: "127.0.0.1:5354"}, nodeFlags: nodeFlags{AcceptDns: false}},
			want: "--no-accept-dns",
		},
		{
			name: "socks5 address without a port",
			cmd:  runCmd{listenFlags: listenFlags{Socks5: "127.0.0.1"}},
			want: "socks5: ",
		},
		{
			name: "http address without a port",
			cmd:  runCmd{listenFlags: listenFlags{HTTP: "127.0.0.1"}},
			want: "http: ",
		},
		{
			name: "dns address without a port",
			cmd:  runCmd{listenFlags: listenFlags{DNS: "127.0.0.1"}, nodeFlags: nodeFlags{AcceptDns: true}},
			want: "dns/udp: ",
		},
		{
			name: "socks5 port already taken",
			cmd:  runCmd{listenFlags: listenFlags{Socks5: busy.Addr().String()}},
			want: "address already in use",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			cmd := tt.cmd
			cmd.StateDir = t.TempDir()
			err := cmd.Run(ctx, log.New(io.Discard, "", 0))
			if err == nil {
				t.Fatalf("Run() = nil, want an error mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Run() = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestFlagEnvVars pins the environment variable of every flag. They are a
// three-place contract with .env.example and the README table, and kong derives
// most of them from the flag name: --socks5 in particular would become
// TSPROXY_SOCKS_5 without its explicit env tag.
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

	var root cli
	parser, err := kong.New(&root, kong.DefaultEnvars("TSPROXY"), kong.Vars{"version": "test"})
	if err != nil {
		t.Fatal(err)
	}
	var run *kong.Node
	for _, child := range parser.Model.Node.Children {
		if child.Name == "run" {
			run = child
		}
	}
	if run == nil {
		t.Fatal(`no "run" command in the model`)
	}

	seen := make(map[string]bool, len(want))
	for _, f := range run.Flags {
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
	for _, f := range parser.Model.Node.Flags {
		if (f.Name == "help" || f.Name == "version") && len(f.Envs) > 0 {
			t.Errorf("--%s reads %q, want no environment variable", f.Name, f.Envs)
		}
	}
}
