package main

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
)

// TestRunRejectsBadFlagsBeforeJoining checks the fail-fast guards in runCmd.Run:
// every case below must be rejected before tsnode.Start is reached, so a typo
// never costs a tailnet login. The context is already cancelled and the state
// directory is throwaway, so a regression cannot quietly join a tailnet here.
func TestRunRejectsBadFlagsBeforeJoining(t *testing.T) {
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
			cmd:  runCmd{DNS: "127.0.0.1:5354", nodeFlags: nodeFlags{AcceptDns: false}},
			want: "--no-accept-dns",
		},
		{
			name: "socks5 address without a port",
			cmd:  runCmd{Socks5: "127.0.0.1"},
			want: "--socks5",
		},
		{
			name: "http address without a port",
			cmd:  runCmd{HTTP: "127.0.0.1"},
			want: "--http",
		},
		{
			name: "dns address without a port",
			cmd:  runCmd{DNS: "127.0.0.1", nodeFlags: nodeFlags{AcceptDns: true}},
			want: "--dns",
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
