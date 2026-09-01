package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// The release is one archive per platform on GitHub, named without a version
// and covered by the SHA256SUMS.txt beside it.
const (
	latestReleaseURL = "https://api.github.com/repos/d0whc3r/tailscale-socks/releases/latest"
	releaseAssetURL  = "https://github.com/d0whc3r/tailscale-socks/releases/download"

	// GitHub answers a request without one with 403.
	userAgent = "tailscale-socks"

	// The archive is read into memory: a stripped Go binary plus a few
	// kilobytes of shell and markdown. The cap is what keeps a wrong URL from
	// filling the machine.
	maxArchive = 200 << 20
)

// upgradeCmd replaces this executable, the zsh helpers and the configuration
// template with the latest release.
type upgradeCmd struct {
	// Endpoints, transport and destinations, so the tests can install a
	// release served by httptest into a temporary directory. Unexported: kong
	// models exported fields only.
	latestURL, assetURL string
	client              *http.Client
	targets             *releaseTargets
}

func (c *upgradeCmd) Run(ctx context.Context) error { return c.run(ctx, os.Stdout) }

func (c *upgradeCmd) run(ctx context.Context, w io.Writer) error {
	if c.latestURL == "" {
		c.latestURL = latestReleaseURL
	}
	if c.assetURL == "" {
		c.assetURL = releaseAssetURL
	}
	if c.client == nil {
		c.client = http.DefaultClient
	}

	name, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if c.targets == nil {
		targets, err := upgradeTargets()
		if err != nil {
			return err
		}
		c.targets = &targets
	}

	tag, err := latestTag(ctx, c.client, c.latestURL)
	if err != nil {
		return err
	}
	current := version()
	if tag == current {
		fmt.Fprintf(w, "version:  %s (already the latest)\n", current)
		return nil
	}

	archive, err := get(ctx, c.client, c.assetURL+"/"+tag+"/"+name)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	sums, err := get(ctx, c.client, c.assetURL+"/"+tag+"/SHA256SUMS.txt")
	if err != nil {
		return fmt.Errorf("downloading SHA256SUMS.txt: %w", err)
	}
	if err := verifyChecksum(archive, sums, name); err != nil {
		return err
	}
	if err := c.targets.install(archive, name); err != nil {
		return err
	}

	fmt.Fprintf(w, "version:  %s -> %s\n", current, tag)
	fmt.Fprintf(w, "binary:   %s\n", c.targets.exe)
	fmt.Fprintf(w, "helpers:  %s\n", filepath.Join(c.targets.shareDir, "contrib", "tailscale-socks.zsh"))
	fmt.Fprintf(w, "config:   %s (your .env is left alone)\n", filepath.Join(c.targets.envDir, ".env.example"))
	return nil
}

// assetName is the archive this platform installs from. macOS has one
// universal build; everywhere else the release carries x86-64 only.
func assetName(goos, goarch string) (string, error) {
	switch {
	case goos == "darwin":
		return "tailscale-socks-darwin-universal.tar.gz", nil
	case goarch != "amd64":
		return "", fmt.Errorf("the release carries no %s/%s build; build from source", goos, goarch)
	case goos == "windows":
		return "tailscale-socks-windows-amd64.zip", nil
	case goos == "linux":
		return "tailscale-socks-linux-amd64.tar.gz", nil
	}
	return "", fmt.Errorf("the release carries no %s build; build from source", goos)
}

// latestTag asks GitHub which release is current. The tag is also the version
// this binary reports, so comparing them needs nothing else.
func latestTag(ctx context.Context, client *http.Client, url string) (string, error) {
	body, err := get(ctx, client, url)
	if err != nil {
		return "", fmt.Errorf("asking GitHub for the latest release: %w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("reading the latest release: %w", err)
	}
	if release.TagName == "" {
		return "", errors.New("the latest release has no tag")
	}
	return release.TagName, nil
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchive))
}

// verifyChecksum matches the archive against its line in SHA256SUMS.txt. Both
// come from the same host over the same TLS, so this is no defence against a
// tampered release — it catches a truncated or corrupted download, which is
// what it is for.
func verifyChecksum(archive, sums []byte, name string) error {
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	for line := range strings.SplitSeq(string(sums), "\n") {
		want, file, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok || file != name {
			continue
		}
		if want != got {
			return fmt.Errorf("%s does not match SHA256SUMS.txt", name)
		}
		return nil
	}
	return fmt.Errorf("%s is not listed in SHA256SUMS.txt", name)
}

// releaseTargets is where an install put the files an upgrade replaces.
type releaseTargets struct {
	exe, shareDir, envDir string
}

// upgradeTargets mirrors contrib/install.sh, including the two directories it
// lets the user move.
func upgradeTargets() (releaseTargets, error) {
	exe, err := os.Executable()
	if err != nil {
		return releaseTargets{}, fmt.Errorf("locating this executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return releaseTargets{}, fmt.Errorf("locating the home directory: %w", err)
	}

	t := releaseTargets{
		exe:      exe,
		shareDir: os.Getenv("TSPROXY_SHARE_DIR"),
		envDir:   os.Getenv("TSPROXY_ENV_DIR"),
	}
	if t.shareDir == "" {
		t.shareDir = filepath.Join(home, ".local", "share", "tailscale-socks")
	}
	if t.envDir == "" {
		t.envDir = filepath.Join(home, ".tailscale")
	}
	return t, nil
}

// install writes the files the archive carries for this platform, each one
// atomically. The .env beside .env.example is never touched: that one is the
// user's, and it may hold TS_AUTHKEY.
func (t releaseTargets) install(archive []byte, name string) error {
	binary := "tailscale-socks"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	type target struct {
		path string
		mode os.FileMode
	}
	want := map[string]target{
		binary: {t.exe, 0o755},
		"contrib/tailscale-socks.zsh": {
			filepath.Join(t.shareDir, "contrib", "tailscale-socks.zsh"), 0o644,
		},
		"contrib/platform/" + runtime.GOOS + ".zsh": {
			filepath.Join(t.shareDir, "contrib", "platform", runtime.GOOS+".zsh"), 0o644,
		},
		".env.example": {filepath.Join(t.envDir, ".env.example"), 0o644},
	}

	visit := func(name string, r io.Reader) error {
		dst, ok := want[name]
		if !ok {
			return nil
		}
		if err := writeFile(dst.path, r, dst.mode, dst.path == t.exe); err != nil {
			return err
		}
		delete(want, name)
		return nil
	}

	var err error
	if strings.HasSuffix(name, ".zip") {
		err = walkZip(archive, visit)
	} else {
		err = walkTarGz(archive, visit)
	}
	if err != nil {
		return fmt.Errorf("unpacking %s: %w", name, err)
	}
	if len(want) > 0 {
		return fmt.Errorf("%s is missing %s", name, strings.Join(slices.Sorted(maps.Keys(want)), ", "))
	}
	return nil
}

func walkTarGz(archive []byte, visit func(string, io.Reader) error) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if err := visit(path.Clean(h.Name), tr); err != nil {
			return err
		}
	}
}

func walkZip(archive []byte, visit func(string, io.Reader) error) error {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		// Closed here rather than deferred: this is a loop, and the reader is
		// finished with as soon as visit returns.
		err = visit(path.Clean(f.Name), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// writeFile replaces dst atomically, through a temporary file in the same
// directory. Windows refuses to overwrite a running executable but does allow
// renaming it away, so when dst is this program the old one is moved aside
// first; it stays locked until this process exits and the next upgrade
// removes it.
func writeFile(dst string, r io.Reader, mode os.FileMode, replacingSelf bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return &upgradeWriteError{dst, err}
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tailscale-socks-*")
	if err != nil {
		return &upgradeWriteError{dst, err}
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return &upgradeWriteError{dst, err}
	}
	if err := tmp.Close(); err != nil {
		return &upgradeWriteError{dst, err}
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return &upgradeWriteError{dst, err}
	}

	if replacingSelf && runtime.GOOS == "windows" {
		old := dst + ".old"
		os.Remove(old)
		if err := os.Rename(dst, old); err != nil {
			return &upgradeWriteError{dst, err}
		}
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return &upgradeWriteError{dst, err}
	}
	return nil
}

// upgradeWriteError names the file an upgrade could not replace, so the hint
// points at that path instead of at the state directory.
type upgradeWriteError struct {
	path string
	err  error
}

func (e *upgradeWriteError) Error() string {
	return fmt.Sprintf("cannot replace %s: %v", e.path, e.err)
}

func (e *upgradeWriteError) Unwrap() error { return e.err }
