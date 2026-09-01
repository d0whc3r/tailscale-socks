package tsnode

import (
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/views"
)

// onlyPeer returns the single peer testStatus builds, so a case can give it an
// ID or a route without rebuilding the whole fixture.
func onlyPeer(t *testing.T, st *ipnstate.Status) *ipnstate.PeerStatus {
	t.Helper()
	if len(st.Peer) != 1 {
		t.Fatalf("testStatus() has %d peers, want exactly 1", len(st.Peer))
	}
	for _, ps := range st.Peer {
		return ps
	}
	return nil
}

// TestDescribe covers the summary `status` prints. describe is split from
// Describe precisely so this can run against a hand-built status, with no
// tailnet and no network.
func TestDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  func(*testing.T, *ipnstate.Status)
		prefs   ipn.Prefs
		want    []string
		notWant []string
	}{
		{
			name: "running node",
			status: func(_ *testing.T, st *ipnstate.Status) {
				st.BackendState = "Running"
				st.Self.DNSName = "proxy.tailnet.ts.net."
			},
			prefs: ipn.Prefs{CorpDNS: true, RouteAll: true},
			want: []string{
				"node:     proxy.tailnet.ts.net (Running)\n",
				"addrs:    100.64.0.1\n",
				"MagicDNS suffix tailnet.ts.net",
				"state:    /state/dir\n",
				"dns:      accept=true\n",
				"routes:   accept=true\n",
				"exit node candidates:\n  - gateway.tailnet.ts.net (online=false)\n",
				"subnet routers: none\n",
			},
			// Health is only worth a line when there is something wrong.
			notWant: []string{"health warnings"},
		},
		{
			name: "control has not answered yet",
			status: func(_ *testing.T, st *ipnstate.Status) {
				st.Self = nil
				st.BackendState = "NoState"
			},
			want: []string{"node:     unknown (NoState)\n", "addrs:    none\n"},
		},
		{
			name: "exit node in use is named, not printed as an ID",
			status: func(t *testing.T, st *ipnstate.Status) {
				onlyPeer(t, st).ID = "nHqLgW"
				st.ExitNodeStatus = &ipnstate.ExitNodeStatus{ID: "nHqLgW", Online: true}
			},
			want:    []string{"exit node: gateway.tailnet.ts.net online=true\n"},
			notWant: []string{"nHqLgW"},
		},
		{
			name:  "auto exit node before one is picked",
			prefs: ipn.Prefs{AutoExitNode: "geo:us"},
			want:  []string{"exit node: auto:geo:us (none selected yet)\n"},
		},
		{
			name: "no exit node",
			want: []string{"exit node: none\n"},
		},
		{
			name: "subnet router",
			status: func(t *testing.T, st *ipnstate.Status) {
				routes := views.SliceOf([]netip.Prefix{
					netip.MustParsePrefix("10.0.0.0/24"),
					netip.MustParsePrefix("192.168.1.0/24"),
				})
				onlyPeer(t, st).PrimaryRoutes = &routes
			},
			want: []string{"subnet routers:\n  - gateway.tailnet.ts.net -> 10.0.0.0/24,192.168.1.0/24\n"},
		},
		{
			name: "health warnings",
			status: func(_ *testing.T, st *ipnstate.Status) {
				st.Health = []string{"not in map poll", "derp region unreachable"}
			},
			want: []string{"health warnings:\n  - not in map poll\n  - derp region unreachable\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := testStatus()
			if tt.status != nil {
				tt.status(t, st)
			}
			prefs := tt.prefs
			got := describe(st, &prefs, "/state/dir")

			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("describe() is missing %q; got:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("describe() mentions %q, want it left out; got:\n%s", w, got)
				}
			}
		})
	}
}

// The summary is read by humans and grepped by contrib/tailscale-socks.zsh:
// one lowercase fact per line, list items indented, no blank lines.
func TestDescribeIsOneFactPerLine(t *testing.T) {
	t.Parallel()

	st := testStatus()
	st.Health = []string{"not in map poll"}
	got := describe(st, &ipn.Prefs{}, "/state/dir")

	if !strings.HasSuffix(got, "\n") {
		t.Errorf("summary does not end in a newline:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if strings.HasPrefix(line, "  - ") {
			continue // list item
		}
		label, _, ok := strings.Cut(line, ":")
		if !ok {
			t.Errorf("line %q is not a `label: value` fact; got:\n%s", line, got)
			continue
		}
		if label != strings.ToLower(label) {
			t.Errorf("label %q is not lowercase; got:\n%s", label, got)
		}
	}
}

// An exit node that is no longer a peer still has to print as something: the
// summary falls back to the raw node ID rather than an empty field.
func TestPeerNameFallsBackToTheID(t *testing.T) {
	t.Parallel()

	if got := peerName(testStatus(), "nosuchnode"); got != "nosuchnode" {
		t.Errorf("peerName(unknown) = %q, want the raw ID", got)
	}
}
