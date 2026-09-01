package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

// report writes err as a single line, followed by one actionable hint when
// there is one. Nothing here prints a stack: the user is meant to read the
// failure and fix it, not to debug this program.
func report(w io.Writer, err error) {
	fmt.Fprintf(w, "tailscale-socks: %v\n", err)
	if h := hint(err); h != "" {
		fmt.Fprintf(w, "hint: %s\n", h)
	}
}

// hint returns what the user can do about err, or "" when the error already
// says everything it can.
func hint(err error) string {
	var op *net.OpError
	if errors.As(err, &op) {
		if op.Op != "listen" || op.Addr == nil {
			return ""
		}
		switch {
		case errors.Is(err, syscall.EADDRINUSE):
			return fmt.Sprintf("another process is already listening on %s; stop it or pick a different address", op.Addr)
		case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
			return fmt.Sprintf("not allowed to bind %s; ports below 1024 need root", op.Addr)
		case errors.Is(err, syscall.EADDRNOTAVAIL):
			return fmt.Sprintf("%s is not an address of this machine", op.Addr)
		}
		return ""
	}

	// An upgrade names the file it could not replace, so the state directory
	// below is the wrong advice for it.
	var uw *upgradeWriteError
	if errors.As(err, &uw) {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Sprintf("%s is not writable by this user; install it under your home with contrib/install.sh", uw.path)
		}
		return ""
	}

	// The state directory is the only other path this program creates, so a
	// file error that got this far is about that directory.
	if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, os.ErrPermission) {
		return "the state directory must be a writable directory; check --state-dir (TSPROXY_STATE_DIR)"
	}
	return ""
}
