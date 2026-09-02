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
	}.config(logf)
	for _, c := range []struct{ field, got, want string }{
		{"Hostname", cfg.Hostname, "box"},
		{"StateDir", cfg.StateDir, "/state"},
		{"AuthKey", cfg.AuthKey, "tskey-auth-notasecretanymore"},
	} {
		if c.got != c.want {
			t.Errorf("config().%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if cfg.Logf == nil {
		t.Error("config() left Logf nil; the login URL would go nowhere")
	}

	// The preferences are the writing half; config() must not carry them, or
	// every caller would push them whether it meant to or not.
	if cfg.Prefs != nil {
		t.Errorf("config() set Prefs to %+v; a node config alone writes nothing", cfg.Prefs)
	}

	// --verbose is the only flag that turns on tsnet's internal chatter.
	if quiet := (nodeFlags{}).config(logf); quiet.DebugLogf != nil {
		t.Error("config() set DebugLogf without --verbose")
	}
	if loud := (nodeFlags{Verbose: true}).config(logf); loud.DebugLogf == nil {
		t.Error("config() with --verbose left DebugLogf nil")
	}
}

// TestPrefFlagsPrefs pins the mapping from the flags onto tsnode.Prefs, the
// half that EditPrefs persists.
func TestPrefFlagsPrefs(t *testing.T) {
	t.Parallel()

	if got := (prefFlags{ExitNode: "auto:any"}).prefs().ExitNode; got != "auto:any" {
		t.Errorf("prefs().ExitNode = %q, want auto:any", got)
	}

	// Exactly one flag on per case, so a field wired to the wrong flag fails.
	tests := []struct {
		name  string
		flags prefFlags
		want  [3]bool // ExitNodeAllowLAN, AcceptRoutes, AcceptDNS
	}{
		{"nothing on", prefFlags{}, [3]bool{false, false, false}},
		{"exit-node-allow-lan", prefFlags{ExitNodeAllowLan: true}, [3]bool{true, false, false}},
		{"accept-routes", prefFlags{AcceptRoutes: true}, [3]bool{false, true, false}},
		{"accept-dns", prefFlags{AcceptDns: true}, [3]bool{false, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := tt.flags.prefs()
			got := [3]bool{p.ExitNodeAllowLAN, p.AcceptRoutes, p.AcceptDNS}
			if got != tt.want {
				t.Errorf("{ExitNodeAllowLAN, AcceptRoutes, AcceptDNS} = %v, want %v", got, tt.want)
			}
		})
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

// TestReportingCommandsTakeNoSettings is the read-only guarantee. status
// writes: EditPrefs persists what it is given, so a status that accepted a
// preference would leave the tailnet reconfigured behind it — and one that
// accepted none but still applied its defaults would clear what run had set.
// config writes nothing, but it is a report all the same, so it answers on the
// environment and the .env files alone.
func TestReportingCommandsTakeNoSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cmd  string
		args []string
	}{
		{"status", []string{"--exit-node", "auto"}},
		{"status", []string{"--exit-node-allow-lan"}},
		{"status", []string{"--accept-routes"}},
		{"status", []string{"--no-accept-routes"}},
		{"status", []string{"--accept-dns"}},
		{"status", []string{"--socks5", "127.0.0.1:1080"}},
		{"config", []string{"--socks5", "127.0.0.1:1080"}},
		{"config", []string{"--hostname", "box"}},
		{"config", []string{"--state-dir", "/state"}},
		{"config", []string{"--exit-node", "auto"}},
		{"config", []string{"--verbose"}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd+" "+tt.args[0], func(t *testing.T) {
			t.Parallel()

			var c cli
			parser, err := kong.New(&c, kong.DefaultEnvars("TSPROXY"), kong.Vars{"version": "test"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.Parse(append([]string{tt.cmd}, tt.args...)); err == nil {
				t.Errorf("%s accepted %s; it reports, it does not configure", tt.cmd, tt.args[0])
			}
		})
	}

	// The other half of status: nothing it hands tsnode can reach EditPrefs.
	if cfg := (statusCmd{}).config(func(string, ...any) {}); cfg.Prefs != nil {
		t.Errorf("status built a config with Prefs %+v", cfg.Prefs)
	}
}

// TestHiddenFlagsStillParse guards what hideSharedFlags must not do: the flags
// status keeps are off its help page, not off its command line. It needs them
// to reach the same node as run.
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
	if _, err := parser.Parse([]string{"status", "--hostname", "box", "--state-dir", "/state"}); err != nil {
		t.Fatalf("status with hidden flags: %v", err)
	}
	if c.Status.Hostname != "box" || c.Status.StateDir != "/state" {
		t.Errorf("status flags = %q/%q, want box//state", c.Status.Hostname, c.Status.StateDir)
	}
}

// TestResolvedRunReadsTheEnvironment is what config now answers on: with no
// command line left to merge, a setting reaches the dump through kong's
// defaults and the environment, or it does not reach it at all.
func TestResolvedRunReadsTheEnvironment(t *testing.T) {
	t.Setenv("TSPROXY_SOCKS5", "127.0.0.1:9050")

	run, err := resolvedRun()
	if err != nil {
		t.Fatalf("resolvedRun() = %v", err)
	}
	if run.Socks5 != "127.0.0.1:9050" {
		t.Errorf("resolvedRun().Socks5 = %q, want the value from the environment", run.Socks5)
	}
	if run.HTTP != "127.0.0.1:8080" {
		t.Errorf("resolvedRun().HTTP = %q, want the default", run.HTTP)
	}
	if !run.AcceptRoutes {
		t.Error("resolvedRun().AcceptRoutes is false; --accept-routes defaults to on")
	}
}
