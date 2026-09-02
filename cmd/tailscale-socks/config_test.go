package main

import (
	"strings"
	"testing"

	"github.com/d0whc3r/tailscale-socks/internal/tsnode"
)

// testRun is a fully resolved run command: every value is set, so a test
// reading one of them cannot pass on a default it did not mean to check. It is
// what config reports on — config itself holds nothing but the key.
func testRun() runCmd {
	return runCmd{
		listenFlags: listenFlags{Socks5: "127.0.0.1:1080", HTTP: "", DNS: "127.0.0.1:5354"},
		nodeFlags:   nodeFlags{Hostname: "ts-proxy", StateDir: "/tmp/state"},
		prefFlags:   prefFlags{ExitNode: "auto", AcceptRoutes: true, AcceptDns: true},
	}
}

func TestConfigKey(t *testing.T) {
	tests := []struct {
		name, key, want string
		edit            func(*runCmd)
	}{
		{name: "flag name", key: "socks5", want: "127.0.0.1:1080"},
		{name: "environment variable", key: "TSPROXY_SOCKS5", want: "127.0.0.1:1080"},
		{name: "mixed case", key: "Exit-Node", want: "auto"},
		{name: "boolean", key: "accept-routes", want: "true"},
		{name: "disabled listener", key: "http", want: ""},
		{
			// The node treats "" and "off" alike; print what the docs say.
			name: "unset exit node",
			key:  "exit-node",
			want: "off",
			edit: func(r *runCmd) { r.ExitNode = "" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := testRun()
			if tt.edit != nil {
				tt.edit(&run)
			}
			cmd := configCmd{Key: tt.key}
			var out strings.Builder
			if err := cmd.write(&out, run); err != nil {
				t.Fatalf("write() = %v", err)
			}
			// A bare value, so $(...) can take it as is.
			if got := strings.TrimSuffix(out.String(), "\n"); got != tt.want {
				t.Errorf("config %s = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestConfigUnknownKey(t *testing.T) {
	cmd := configCmd{Key: "sock5"}
	var out strings.Builder
	err := cmd.write(&out, testRun())
	if err == nil {
		t.Fatalf("write() = nil, want an error")
	}
	// The message has to be enough to fix the typo without reading --help.
	if !strings.Contains(err.Error(), "socks5") {
		t.Errorf("write() = %v, want it to list the valid settings", err)
	}
	if out.Len() != 0 {
		t.Errorf("write() also printed %q, want nothing", out.String())
	}
}

func TestConfigDump(t *testing.T) {
	run := testRun()
	run.StateDir = "/tmp/we'ird dir"
	var out strings.Builder
	if err := (&configCmd{}).write(&out, run); err != nil {
		t.Fatalf("write() = %v", err)
	}
	want := []string{
		"TSPROXY_HOSTNAME='ts-proxy'\n",
		`TSPROXY_STATE_DIR='/tmp/we'\''ird dir'` + "\n",
		"TSPROXY_HTTP=''\n",
		"TSPROXY_ACCEPT_DNS='true'\n",
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Errorf("dump is missing %q; got:\n%s", w, out.String())
		}
	}
}

// The dump is meant to be printed, piped and eval'd, so the one value that
// must never reach it is the auth key.
func TestConfigNeverPrintsTheAuthKey(t *testing.T) {
	const secret = "tskey-auth-notasecretanymore"
	run := testRun()
	run.AuthKey = secret
	var out strings.Builder
	if err := (&configCmd{}).write(&out, run); err != nil {
		t.Fatalf("write() = %v", err)
	}
	if strings.Contains(out.String(), secret) {
		t.Errorf("dump leaked the auth key:\n%s", out.String())
	}
}

// The state directory is the only setting config has to derive itself.
func TestConfigDefaultsTheStateDirToTheHostname(t *testing.T) {
	run := testRun()
	run.StateDir = ""
	cmd := configCmd{Key: "state-dir"}
	var out strings.Builder
	if err := cmd.write(&out, run); err != nil {
		t.Fatalf("write() = %v", err)
	}
	want, err := tsnode.DefaultStateDir(run.Hostname)
	if err != nil {
		t.Fatalf("DefaultStateDir() = %v", err)
	}
	if got := strings.TrimSuffix(out.String(), "\n"); got != want {
		t.Errorf("config state-dir = %q, want %q", got, want)
	}
}

// The config dump is the fourth place a flag has to be registered, and the
// only one no other test covers: a flag that never reaches settingsFor() is
// silently missing from `tailscale-socks config`. The auth key is the
// deliberate exception, being a credential.
func TestConfigSettingsCoverEveryFlag(t *testing.T) {
	t.Parallel()

	settings, err := settingsFor(testRun())
	if err != nil {
		t.Fatalf("settingsFor() = %v", err)
	}
	envByFlag := make(map[string]string, len(settings))
	for _, s := range settings {
		envByFlag[s.flag] = s.env
	}

	run, _ := cliFlags(t)
	flags := make(map[string]bool, len(run))
	for _, f := range run {
		flags[f.Name] = true
		if f.Name == "auth-key" {
			if _, ok := envByFlag[f.Name]; ok {
				t.Error("settingsFor() exposes the auth key; it is a credential")
			}
			continue
		}
		env, ok := envByFlag[f.Name]
		if !ok {
			t.Errorf("--%s is missing from settingsFor()", f.Name)
			continue
		}
		if len(f.Envs) > 0 && env != f.Envs[0] {
			t.Errorf("settingsFor() reports --%s as %s, want %s", f.Name, env, f.Envs[0])
		}
	}
	for flag := range envByFlag {
		if !flags[flag] {
			t.Errorf("settingsFor() reports %q, which is not a flag any more", flag)
		}
	}
}
