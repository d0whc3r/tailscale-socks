package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

// listenErr rebuilds the error net.Listen returns for a failed bind.
func listenErr(addr string, errno syscall.Errno) error {
	tcp, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	return &net.OpError{Op: "listen", Net: "tcp", Addr: tcp, Err: &os.SyscallError{Syscall: "bind", Err: errno}}
}

func TestHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string // substring; "" means no hint at all
	}{
		{"address in use", listenErr("127.0.0.1:1080", syscall.EADDRINUSE), "already listening on 127.0.0.1:1080"},
		{"privileged port", listenErr("127.0.0.1:53", syscall.EACCES), "ports below 1024 need root"},
		{"not our address", listenErr("10.0.0.1:1080", syscall.EADDRNOTAVAIL), "not an address of this machine"},
		{"wrapped by the caller", fmt.Errorf("socks5: %w", listenErr("127.0.0.1:1080", syscall.EADDRINUSE)), "already listening"},
		{"other listen failure", listenErr("127.0.0.1:1080", syscall.EINVAL), ""},
		{"dial failure", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, ""},
		{"state dir is a file", fmt.Errorf("joining tailnet: %w", &os.PathError{Op: "mkdir", Path: "/tmp/x", Err: syscall.ENOTDIR}), "--state-dir"},
		{"state dir not writable", fmt.Errorf("joining tailnet: %w", &os.PathError{Op: "mkdir", Path: "/x", Err: syscall.EACCES}), "--state-dir"},
		{"nothing to add", errors.New("nothing to serve"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hint(tt.err)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("hint() = %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("hint() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestReportIsOneLinePerFact(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	report(&b, fmt.Errorf("socks5: %w", listenErr("127.0.0.1:1080", syscall.EADDRINUSE)))

	want := "tailscale-socks: socks5: listen tcp 127.0.0.1:1080: bind: address already in use\n" +
		"hint: another process is already listening on 127.0.0.1:1080; stop it or pick a different address\n"
	if b.String() != want {
		t.Fatalf("report() =\n%q\nwant\n%q", b.String(), want)
	}

	b.Reset()
	report(&b, errors.New("nothing to serve"))
	if got := b.String(); got != "tailscale-socks: nothing to serve\n" {
		t.Fatalf("report() = %q", got)
	}
}
