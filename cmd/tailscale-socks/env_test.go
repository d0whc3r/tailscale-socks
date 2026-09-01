package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeEnvFile(t *testing.T, dir, body string, perm os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDotEnvsPrecedence(t *testing.T) {
	next := writeEnvFile(t, t.TempDir(), "TSPROXY_SOCKS5=from-exe\nTSPROXY_HTTP=from-exe\n", 0o600)
	home := writeEnvFile(t, t.TempDir(), "TSPROXY_SOCKS5=from-home\nTSPROXY_DNS=from-home\n", 0o600)

	// Already in the environment: must survive both files.
	t.Setenv("TSPROXY_HTTP", "from-env")
	t.Setenv("TSPROXY_SOCKS5", "")
	os.Unsetenv("TSPROXY_SOCKS5")
	t.Setenv("TSPROXY_DNS", "")
	os.Unsetenv("TSPROXY_DNS")

	loadDotEnvs([]string{next, home, filepath.Join(t.TempDir(), "missing.env")})

	for _, tt := range []struct{ key, want string }{
		{"TSPROXY_HTTP", "from-env"},   // environment wins over every file
		{"TSPROXY_SOCKS5", "from-exe"}, // first file wins over later ones
		{"TSPROXY_DNS", "from-home"},   // only set by the later file
	} {
		if got := os.Getenv(tt.key); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLoadDotEnvsWarnsOnLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no permission bits to warn about: os.Stat synthesises 0666 on Windows")
	}
	loose := writeEnvFile(t, t.TempDir(), "TS_AUTHKEY=tskey-auth-secret\n", 0o644)
	msgs := loadDotEnvs([]string{loose})
	t.Setenv("TS_AUTHKEY", "") // do not leak into other tests

	var warned bool
	for _, m := range msgs {
		if strings.HasPrefix(m, "warning:") && strings.Contains(m, loose) {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no permission warning for a world-readable .env; got %q", msgs)
	}
}

func TestDotEnvPathsOrder(t *testing.T) {
	paths := dotEnvPaths()
	if len(paths) != 2 {
		t.Fatalf("dotEnvPaths() = %q, want the executable's .env then ~/.tailscale/.env", paths)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(filepath.Dir(paths[0])), filepath.Base(filepath.Dir(exe)); got != want {
		t.Errorf("first path is in %q, want the executable's directory %q", got, want)
	}
	if want := filepath.Join(".tailscale", ".env"); !strings.HasSuffix(paths[1], want) {
		t.Errorf("second path = %q, want it to end in %q", paths[1], want)
	}
}
