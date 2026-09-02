package main

import (
	"bytes"
	"strings"
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

// TestNodeFlagsConfig pins the mapping from the flags onto tsnode.Config. This
// is where a field wired to the wrong flag stops being visible: the node comes
// up fine and quietly ignores what the user asked for.
func TestNodeFlagsConfig(t *testing.T) {
	t.Parallel()

	logf := func(string, ...any) {}

	// The strings are a straight copy, so one case covers them.
	cfg := nodeFlags{
		Hostname: "box",
		StateDir: "/state",
		AuthKey:  "tskey-auth-notasecretanymore",
		ExitNode: "auto:any",
	}.config(logf)
	for _, c := range []struct{ field, got, want string }{
		{"Hostname", cfg.Hostname, "box"},
		{"StateDir", cfg.StateDir, "/state"},
		{"AuthKey", cfg.AuthKey, "tskey-auth-notasecretanymore"},
		{"ExitNode", cfg.ExitNode, "auto:any"},
	} {
		if c.got != c.want {
			t.Errorf("config().%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if cfg.Logf == nil {
		t.Error("config() left Logf nil; the login URL would go nowhere")
	}

	// Exactly one flag on per case, so a field wired to the wrong flag fails.
	tests := []struct {
		name  string
		flags nodeFlags
		want  [3]bool // ExitNodeAllowLAN, AcceptRoutes, AcceptDNS
	}{
		{"nothing on", nodeFlags{}, [3]bool{false, false, false}},
		{"exit-node-allow-lan", nodeFlags{ExitNodeAllowLan: true}, [3]bool{true, false, false}},
		{"accept-routes", nodeFlags{AcceptRoutes: true}, [3]bool{false, true, false}},
		{"accept-dns", nodeFlags{AcceptDns: true}, [3]bool{false, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.flags.config(logf)
			got := [3]bool{cfg.ExitNodeAllowLAN, cfg.AcceptRoutes, cfg.AcceptDNS}
			if got != tt.want {
				t.Errorf("{ExitNodeAllowLAN, AcceptRoutes, AcceptDNS} = %v, want %v", got, tt.want)
			}
		})
	}

	// --verbose is the only flag that turns on tsnet's internal chatter.
	if quiet := (nodeFlags{}).config(logf); quiet.DebugLogf != nil {
		t.Error("config() set DebugLogf without --verbose")
	}
	if loud := (nodeFlags{Verbose: true}).config(logf); loud.DebugLogf == nil {
		t.Error("config() with --verbose left DebugLogf nil")
	}
}

// help renders one command's help page the way main does, and returns it.
func help(t *testing.T, args ...string) string {
	t.Helper()

	var c cli
	var out bytes.Buffer
	parser, err := kong.New(&c,
		kong.Name("tailscale-socks"),
		kong.DefaultEnvars("TSPROXY"),
		kong.Description(description),
		kong.Help(helpPrinter),
		kong.PostBuild(hideSharedFlags),
		kong.Vars{"version": "test"},
		kong.Writers(&out, &out),
		kong.Exit(func(int) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
	// kong.Exit is a no-op here, so parsing runs on past --help and may then
	// complain about a missing command; the help is already out.
	_, _ = parser.Parse(append(args, "--help"))
	return out.String()
}

// TestHelpFlagsPerCommand pins the help pages. The node and listen flags reach
// status and config too, but only run is about them, so only run lists them —
// bar --verbose, which stays on status because status also brings the node up.
// Only a page that lists a flag keeps kong's "[flags]".
func TestHelpFlagsPerCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		want     []string
		unwanted []string
	}{
		{
			name:     "root",
			want:     []string{"Usage: tailscale-socks <command> [flags]", "run [flags]", "status [flags]"},
			unwanted: []string{"config [<key>] [flags]", "upgrade [flags]", "--hostname"},
		},
		{
			name: "run",
			args: []string{"run"},
			want: []string{"Usage: tailscale-socks run [flags]", "--socks5", "--hostname", "--[no-]accept-dns"},
		},
		{
			name:     "status",
			args:     []string{"status"},
			want:     []string{"Usage: tailscale-socks status [flags]", "-v, --verbose"},
			unwanted: []string{"--hostname", "--exit-node", "--socks5"},
		},
		{
			name:     "config",
			args:     []string{"config"},
			want:     []string{"Usage: tailscale-socks config [<key>]\n"},
			unwanted: []string{"[flags]", "--socks5", "--hostname"},
		},
		{
			name:     "upgrade",
			args:     []string{"upgrade"},
			want:     []string{"Usage: tailscale-socks upgrade\n"},
			unwanted: []string{"[flags]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := help(t, tt.args...)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("help is missing %q:\n%s", w, got)
				}
			}
			for _, u := range tt.unwanted {
				if strings.Contains(got, u) {
					t.Errorf("help still shows %q:\n%s", u, got)
				}
			}
		})
	}
}

// TestHiddenFlagsStillParse guards what hideSharedFlags must not do: the flags
// are off the help page of status and config, not off the command line. status
// needs them to reach the same node as run, and config is the one command that
// exists to resolve them.
func TestHiddenFlagsStillParse(t *testing.T) {
	t.Parallel()

	var c cli
	parser, err := kong.New(&c,
		kong.DefaultEnvars("TSPROXY"),
		kong.PostBuild(hideSharedFlags),
		kong.Vars{"version": "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"status", "--hostname", "box", "--exit-node", "auto"}); err != nil {
		t.Fatalf("status with hidden flags: %v", err)
	}
	if c.Status.Hostname != "box" || c.Status.ExitNode != "auto" {
		t.Errorf("status flags = %q/%q, want box/auto", c.Status.Hostname, c.Status.ExitNode)
	}
	if _, err := parser.Parse([]string{"config", "--socks5", "127.0.0.1:9050", "socks5"}); err != nil {
		t.Fatalf("config with hidden flags: %v", err)
	}
	if c.Config.Socks5 != "127.0.0.1:9050" {
		t.Errorf("config --socks5 = %q, want 127.0.0.1:9050", c.Config.Socks5)
	}
}
