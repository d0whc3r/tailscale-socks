package main

import (
	"context"
	"io"
	"log"
	"net"
	"strings"
	"testing"
)

// TestRunRejectsBadFlagsBeforeJoining checks the fail-fast guards in runCmd.Run:
// every case below must be rejected before tsnode.Start is reached, so a typo or
// a busy port never costs a tailnet login. The context is already cancelled and
// the state directory is throwaway, so a regression cannot quietly join a
// tailnet here.
func TestRunRejectsBadFlagsBeforeJoining(t *testing.T) {
	// A port this process already owns: binding it again must fail.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup, not defer: the parallel subtests below run after this
	// function returns, and a deferred Close would free the port first.
	t.Cleanup(func() { busy.Close() })

	tests := []struct {
		name string
		cmd  runCmd
		want string
	}{
		{
			name: "every listener disabled",
			cmd:  runCmd{},
			want: "nothing to serve",
		},
		{
			name: "dns server without tailnet dns",
			cmd:  runCmd{listenFlags: listenFlags{DNS: "127.0.0.1:5354"}, nodeFlags: nodeFlags{AcceptDns: false}},
			want: "--no-accept-dns",
		},
		{
			name: "socks5 address without a port",
			cmd:  runCmd{listenFlags: listenFlags{Socks5: "127.0.0.1"}},
			want: "socks5: ",
		},
		{
			name: "http address without a port",
			cmd:  runCmd{listenFlags: listenFlags{HTTP: "127.0.0.1"}},
			want: "http: ",
		},
		{
			name: "dns address without a port",
			cmd:  runCmd{listenFlags: listenFlags{DNS: "127.0.0.1"}, nodeFlags: nodeFlags{AcceptDns: true}},
			want: "dns/udp: ",
		},
		{
			name: "socks5 port already taken",
			cmd:  runCmd{listenFlags: listenFlags{Socks5: busy.Addr().String()}},
			want: "address already in use",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			cmd := tt.cmd
			cmd.StateDir = t.TempDir()
			err := cmd.Run(ctx, log.New(io.Discard, "", 0))
			if err == nil {
				t.Fatalf("Run() = nil, want an error mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Run() = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
