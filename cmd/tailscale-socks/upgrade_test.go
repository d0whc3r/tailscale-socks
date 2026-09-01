package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releaseFiles is the payload GoReleaser puts in every archive, keyed by the
// path inside it. README.md is there to prove the extra files are ignored.
func releaseFiles() map[string]string {
	binary := "tailscale-socks"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	return map[string]string{
		binary:                        "new binary",
		"contrib/tailscale-socks.zsh": "new helpers",
		"contrib/platform/" + runtime.GOOS + ".zsh": "new backend",
		".env.example": "new template",
		"README.md":    "not installed",
	}
}

// releaseArchive builds that payload in the format the asset name asks for.
func releaseArchive(t *testing.T, name string) []byte {
	t.Helper()

	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		zw := zip.NewWriter(&buf)
		for path, body := range releaseFiles() {
			w, err := zw.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for path, body := range releaseFiles() {
		// "./" prefixed, as tar -C dir . writes it: the walker has to clean
		// the name before matching.
		hdr := &tar.Header{Name: "./" + path, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testTargets points an upgrade at three temporary directories, with the
// files an earlier install would have left there.
func testTargets(t *testing.T) releaseTargets {
	t.Helper()

	dir := t.TempDir()
	tg := releaseTargets{
		exe:      filepath.Join(dir, "bin", "tailscale-socks"),
		shareDir: filepath.Join(dir, "share"),
		envDir:   filepath.Join(dir, "env"),
	}
	if runtime.GOOS == "windows" {
		tg.exe += ".exe"
	}
	for _, d := range []string{filepath.Dir(tg.exe), tg.shareDir, tg.envDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(tg.exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The user's own file. An upgrade must not touch it: it may hold
	// TS_AUTHKEY.
	if err := os.WriteFile(filepath.Join(tg.envDir, ".env"), []byte("TS_AUTHKEY=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return tg
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAssetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{"darwin", "arm64", "tailscale-socks-darwin-universal.tar.gz", false},
		{"darwin", "amd64", "tailscale-socks-darwin-universal.tar.gz", false},
		{"linux", "amd64", "tailscale-socks-linux-amd64.tar.gz", false},
		{"windows", "amd64", "tailscale-socks-windows-amd64.zip", false},
		{"linux", "arm64", "", true},
		{"freebsd", "amd64", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			t.Parallel()

			got, err := assetName(tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("assetName(%q, %q) = %q, want an error", tt.goos, tt.goarch, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("assetName(%q, %q) = %v", tt.goos, tt.goarch, err)
			}
			if got != tt.want {
				t.Errorf("assetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestLatestTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{name: "a tagged release", status: http.StatusOK, body: `{"tag_name":"v1.2.3"}`, want: "v1.2.3"},
		{name: "extra fields are ignored", status: http.StatusOK, body: `{"tag_name":"v1.2.3","name":"x"}`, want: "v1.2.3"},
		{name: "no releases yet", status: http.StatusNotFound, body: `{}`, wantErr: true},
		{name: "not json", status: http.StatusOK, body: `<html>`, wantErr: true},
		{name: "no tag", status: http.StatusOK, body: `{}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") == "" {
					t.Error("request has no User-Agent; GitHub answers those with 403")
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			got, err := latestTag(t.Context(), srv.Client(), srv.URL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("latestTag() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("latestTag() = %v", err)
			}
			if got != tt.want {
				t.Errorf("latestTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	t.Parallel()

	archive := []byte("an archive")
	sum := sha256.Sum256(archive)
	line := hex.EncodeToString(sum[:]) + "  tailscale-socks-linux-amd64.tar.gz\n"

	tests := []struct {
		name    string
		sums    string
		wantErr bool
	}{
		{name: "the file is listed", sums: "aa  other.tar.gz\n" + line},
		{name: "a different digest", sums: strings.Repeat("0", 64) + "  tailscale-socks-linux-amd64.tar.gz\n", wantErr: true},
		{name: "the file is missing", sums: "aa  other.tar.gz\n", wantErr: true},
		{name: "nothing at all", sums: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := verifyChecksum(archive, []byte(tt.sums), "tailscale-socks-linux-amd64.tar.gz")
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyChecksum() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstallReplacesEveryReleaseFile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"tailscale-socks-linux-amd64.tar.gz", "tailscale-socks-windows-amd64.zip"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tg := testTargets(t)
			if err := tg.install(releaseArchive(t, name), name); err != nil {
				t.Fatalf("install() = %v", err)
			}

			if got := readFile(t, tg.exe); got != "new binary" {
				t.Errorf("binary = %q, want %q", got, "new binary")
			}
			helpers := filepath.Join(tg.shareDir, "contrib", "tailscale-socks.zsh")
			if got := readFile(t, helpers); got != "new helpers" {
				t.Errorf("helpers = %q, want %q", got, "new helpers")
			}
			backend := filepath.Join(tg.shareDir, "contrib", "platform", runtime.GOOS+".zsh")
			if got := readFile(t, backend); got != "new backend" {
				t.Errorf("backend = %q, want %q", got, "new backend")
			}
			if got := readFile(t, filepath.Join(tg.envDir, ".env.example")); got != "new template" {
				t.Errorf(".env.example = %q, want %q", got, "new template")
			}

			// The two things an upgrade must never do.
			if got := readFile(t, filepath.Join(tg.envDir, ".env")); got != "TS_AUTHKEY=secret\n" {
				t.Errorf(".env = %q, want it untouched", got)
			}
			if _, err := os.Stat(filepath.Join(tg.shareDir, "README.md")); err == nil {
				t.Error("README.md was installed; only the four files belong")
			}

			if runtime.GOOS != "windows" {
				fi, err := os.Stat(tg.exe)
				if err != nil {
					t.Fatal(err)
				}
				if fi.Mode().Perm() != 0o755 {
					t.Errorf("binary mode = %v, want 0755", fi.Mode().Perm())
				}
			}
		})
	}
}

func TestInstallRejectsAnArchiveMissingTheBinary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("new helpers")
	if err := tw.WriteHeader(&tar.Header{Name: "contrib/tailscale-socks.zsh", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	tg := testTargets(t)
	err := tg.install(buf.Bytes(), "tailscale-socks-linux-amd64.tar.gz")
	if err == nil {
		t.Fatal("install() = nil, want an error naming what is missing")
	}
	if !strings.Contains(err.Error(), ".env.example") {
		t.Errorf("install() = %v, want it to name the missing files", err)
	}
	if got := readFile(t, tg.exe); got != "old binary" {
		t.Errorf("binary = %q, want the old one still in place", got)
	}
}

// testRelease serves what an upgrade fetches: the tag, the archive and the
// checksums that cover it.
func testRelease(t *testing.T, tag, name string) (*httptest.Server, []byte) {
	t.Helper()

	archive := releaseArchive(t, name)
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"

	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc("/download/"+tag+"/"+name, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/download/"+tag+"/SHA256SUMS.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, sums)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, archive
}

func TestUpgradeRunInstallsTheLatestRelease(t *testing.T) {
	t.Parallel()

	name, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no release for this platform: %v", err)
	}
	srv, _ := testRelease(t, "v9.9.9", name)
	tg := testTargets(t)

	cmd := &upgradeCmd{
		latestURL: srv.URL + "/latest",
		assetURL:  srv.URL + "/download",
		client:    srv.Client(),
		targets:   &tg,
	}
	var out bytes.Buffer
	if err := cmd.run(t.Context(), &out); err != nil {
		t.Fatalf("run() = %v", err)
	}

	if got := readFile(t, tg.exe); got != "new binary" {
		t.Errorf("binary = %q, want %q", got, "new binary")
	}
	for _, want := range []string{"v9.9.9", tg.exe, ".env.example"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q does not mention %q", out.String(), want)
		}
	}
}

func TestUpgradeRunChangesNothingWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	name, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no release for this platform: %v", err)
	}
	srv, _ := testRelease(t, version(), name)
	tg := testTargets(t)

	cmd := &upgradeCmd{
		latestURL: srv.URL + "/latest",
		assetURL:  srv.URL + "/download",
		client:    srv.Client(),
		targets:   &tg,
	}
	var out bytes.Buffer
	if err := cmd.run(t.Context(), &out); err != nil {
		t.Fatalf("run() = %v", err)
	}

	if got := readFile(t, tg.exe); got != "old binary" {
		t.Errorf("binary = %q, want it left alone", got)
	}
	if !strings.Contains(out.String(), "already the latest") {
		t.Errorf("output = %q, want it to say the version is current", out.String())
	}
}

func TestUpgradeRunRefusesAMismatchedChecksum(t *testing.T) {
	t.Parallel()

	name, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no release for this platform: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v9.9.9"}`)
	})
	mux.HandleFunc("/download/v9.9.9/"+name, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(releaseArchive(t, name))
	})
	mux.HandleFunc("/download/v9.9.9/SHA256SUMS.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tg := testTargets(t)
	cmd := &upgradeCmd{
		latestURL: srv.URL + "/latest",
		assetURL:  srv.URL + "/download",
		client:    srv.Client(),
		targets:   &tg,
	}
	if err := cmd.run(t.Context(), &bytes.Buffer{}); err == nil {
		t.Fatal("run() = nil, want it to refuse the archive")
	}
	if got := readFile(t, tg.exe); got != "old binary" {
		t.Errorf("binary = %q, want it left alone", got)
	}
}

func TestUpgradeTargetsFollowsTheInstallerVariables(t *testing.T) {
	t.Setenv("TSPROXY_SHARE_DIR", filepath.Join("somewhere", "share"))
	t.Setenv("TSPROXY_ENV_DIR", filepath.Join("somewhere", "env"))

	tg, err := upgradeTargets()
	if err != nil {
		t.Fatalf("upgradeTargets() = %v", err)
	}
	if tg.shareDir != filepath.Join("somewhere", "share") {
		t.Errorf("shareDir = %q, want the one from the environment", tg.shareDir)
	}
	if tg.envDir != filepath.Join("somewhere", "env") {
		t.Errorf("envDir = %q, want the one from the environment", tg.envDir)
	}
	if tg.exe == "" {
		t.Error("exe is empty; it must be this executable")
	}
}
