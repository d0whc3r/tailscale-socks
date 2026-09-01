package tsnode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"tailscale.com/tsnet"
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

// TestPrefsFor pins the mapping from Config onto the preferences the node is
// sent. Two ways to get this subtly wrong: wire a field to the wrong flag, or
// forget its Set flag — then EditPrefs keeps the value from the last run and
// the flag silently does nothing.
func TestPrefsFor(t *testing.T) {
	t.Parallel()

	// Exactly one flag on per case, so a field wired to the wrong flag fails.
	tests := []struct {
		name string
		cfg  Config
		want [3]bool // RouteAll, CorpDNS, ExitNodeAllowLANAccess
	}{
		{"nothing on", Config{}, [3]bool{false, false, false}},
		{"accept-routes", Config{AcceptRoutes: true}, [3]bool{true, false, false}},
		{"accept-dns", Config{AcceptDNS: true}, [3]bool{false, true, false}},
		{"exit-node-allow-lan", Config{ExitNodeAllowLAN: true}, [3]bool{false, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mp, err := prefsFor(tt.cfg, testStatus())
			if err != nil {
				t.Fatalf("prefsFor() = %v", err)
			}
			got := [3]bool{mp.RouteAll, mp.CorpDNS, mp.ExitNodeAllowLANAccess}
			if got != tt.want {
				t.Errorf("{RouteAll, CorpDNS, ExitNodeAllowLANAccess} = %v, want %v", got, tt.want)
			}
			for name, set := range map[string]bool{
				"RouteAllSet":               mp.RouteAllSet,
				"CorpDNSSet":                mp.CorpDNSSet,
				"ExitNodeAllowLANAccessSet": mp.ExitNodeAllowLANAccessSet,
				"ExitNodeIDSet":             mp.ExitNodeIDSet,
				"ExitNodeIPSet":             mp.ExitNodeIPSet,
				"AutoExitNodeSet":           mp.AutoExitNodeSet,
			} {
				if !set {
					t.Errorf("%s is false; EditPrefs would ignore the flag", name)
				}
			}
		})
	}
}

// A bad --exit-node has to reach the caller: it is the one mapping that can
// fail, and failing before EditPrefs is what keeps a typo from being applied.
func TestPrefsForPropagatesABadExitNode(t *testing.T) {
	t.Parallel()

	if _, err := prefsFor(Config{ExitNode: "nosuchpeer"}, testStatus()); err == nil {
		t.Error("prefsFor() accepted an unknown exit node")
	}
}

// TestCloseStartedSkipsAnUnbuiltServer guards the nil dereference in tsnet's own
// Close: it uses subsystems that only exist once the start sequence got far
// enough, so closing in that window panics and buries the real error. The test
// asserts by not panicking; Sys() is a plain field read, safe here.
func TestCloseStartedSkipsAnUnbuiltServer(t *testing.T) {
	t.Parallel()

	closeStarted(&tsnet.Server{})
}

// fakeConn stands in for a tailnet connection: DialContext only ever passes it
// back, so nothing calls a method on it.
type fakeConn struct{ net.Conn }

// TestDialContext covers the routing every proxy dials through. The two seams
// on Node are filled in by hand here, so none of this needs a tailnet.
func TestDialContext(t *testing.T) {
	t.Parallel()

	const name = "peer.tailnet.ts.net"
	v4, v6 := "100.64.0.2:80", "[fd7a::2]:80"
	ips := []netip.Addr{netip.MustParseAddr("100.64.0.2"), netip.MustParseAddr("fd7a::2")}

	tests := []struct {
		name        string
		addr        string
		ips         []netip.Addr
		lookupErr   error
		accept      []string // addresses the fake dialer lets through
		wantTried   []string // in order
		wantResolve bool
		wantErr     []string // substrings; empty means the dial must succeed
	}{
		{
			// The whole point of the proxies: a literal address is never sent
			// back through the resolver.
			name:      "an IP is dialed verbatim, never resolved",
			addr:      "100.64.0.2:80",
			accept:    []string{v4},
			wantTried: []string{v4},
		},
		{
			name:        "a name resolves and the first address wins",
			addr:        name + ":80",
			ips:         ips,
			accept:      []string{v4},
			wantTried:   []string{v4},
			wantResolve: true,
		},
		{
			name:        "every resolved address is tried, in order",
			addr:        name + ":80",
			ips:         ips,
			accept:      []string{v6},
			wantTried:   []string{v4, v6},
			wantResolve: true,
		},
		{
			// tsnet's own dialer still knows netmap names and the host
			// resolver, so a failed lookup must not fail the dial outright.
			name:        "a failed lookup falls back to the unresolved name",
			addr:        name + ":80",
			lookupErr:   ErrNoDNS,
			accept:      []string{name + ":80"},
			wantTried:   []string{name + ":80"},
			wantResolve: true,
		},
		{
			name:        "an empty answer falls back to the unresolved name",
			addr:        name + ":80",
			accept:      []string{name + ":80"},
			wantTried:   []string{name + ":80"},
			wantResolve: true,
		},
		{
			name:        "every address refused reports all of them",
			addr:        name + ":80",
			ips:         ips,
			wantTried:   []string{v4, v6},
			wantResolve: true,
			wantErr:     []string{"dial " + name + ":80", "refused " + v4, "refused " + v6},
		},
		{
			name:    "an address without a port is rejected before dialing",
			addr:    name,
			wantErr: []string{"missing port"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accept := make(map[string]bool, len(tt.accept))
			for _, a := range tt.accept {
				accept[a] = true
			}
			var tried []string
			var resolved bool
			n := &Node{
				dialTS: func(_ context.Context, _, addr string) (net.Conn, error) {
					tried = append(tried, addr)
					if !accept[addr] {
						return nil, fmt.Errorf("refused %s", addr)
					}
					return fakeConn{}, nil
				},
				lookupIP: func(_ context.Context, host string) ([]netip.Addr, error) {
					resolved = true
					if host != name {
						t.Errorf("lookupIP(%q), want %q", host, name)
					}
					return tt.ips, tt.lookupErr
				},
			}

			conn, err := n.DialContext(context.Background(), "tcp", tt.addr)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("DialContext() = %v, want a connection", err)
				}
				if conn == nil {
					t.Fatal("DialContext() returned no connection and no error")
				}
			} else {
				if err == nil {
					t.Fatalf("DialContext() = nil, want an error mentioning %q", tt.wantErr)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("DialContext() = %v, want it to mention %q", err, w)
					}
				}
			}
			if !slices.Equal(tried, tt.wantTried) {
				t.Errorf("dialed %v, want %v", tried, tt.wantTried)
			}
			if resolved != tt.wantResolve {
				t.Errorf("resolved = %t, want %t", resolved, tt.wantResolve)
			}
		})
	}
}
