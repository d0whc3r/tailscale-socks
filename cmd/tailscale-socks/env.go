package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// dotEnvPaths lists the .env files to read, highest priority first: the one
// next to the executable, then ~/.tailscale/.env.
func dotEnvPaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".tailscale", ".env"))
	}
	return paths
}

// loadDotEnvs reads paths in order into the process environment and returns
// messages worth showing to the user.
//
// Nothing already set in the environment is overwritten, so the precedence is:
// command line > environment > first path > later paths. Missing files are
// skipped silently.
func loadDotEnvs(paths []string) (messages []string) {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		// These files may hold TS_AUTHKEY, which is a credential. Windows
		// has no such bits — os.Stat synthesises 0666 there, so the check
		// would warn on every run and point at a chmod that does nothing.
		if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
			messages = append(messages, fmt.Sprintf("warning: %s is readable by other users; chmod 600 it (it may hold TS_AUTHKEY)", p))
		}
		if err := godotenv.Load(p); err != nil {
			messages = append(messages, fmt.Sprintf("warning: %s: %v", p, err))
			continue
		}
		messages = append(messages, "config: loaded "+p)
	}
	return messages
}
