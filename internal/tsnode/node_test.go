package tsnode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStartWithUnusableStateDirFails(t *testing.T) {
	t.Parallel()

	// A regular file where the state directory's parent should be: tsnet
	// cannot create the directory, so this never reaches the network.
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Start used to hand the half-built node to tsnet's Close, which
	// dereferences subsystems that do not exist yet, so bound the wait
	// instead of hanging the whole run.
	done := make(chan error, 1)
	go func() {
		node, err := Start(context.Background(), Config{
			Hostname: "test-node",
			StateDir: filepath.Join(blocked, "state"),
		})
		if node != nil {
			node.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Start succeeded with an unusable state dir")
		}
		if !errors.Is(err, syscall.ENOTDIR) {
			t.Fatalf("Start error = %v, want it to wrap ENOTDIR", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not return")
	}
}
