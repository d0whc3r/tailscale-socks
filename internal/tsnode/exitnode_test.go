package tsnode

import (
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func testStatus() *ipnstate.Status {
	nodeKey := key.NewNode().Public()
	return &ipnstate.Status{
		CurrentTailnet: &ipnstate.TailnetStatus{MagicDNSSuffix: "tailnet.ts.net"},
		// ipn.exitNodeIPOfArg still resolves short peer names against the
		// legacy field, so it has to be set for the "gateway" case to work.
		//lint:ignore SA1019 upstream still reads this field
		MagicDNSSuffix: "tailnet.ts.net",
		Self:           &ipnstate.PeerStatus{TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			nodeKey: {
				DNSName:        "gateway.tailnet.ts.net.",
				TailscaleIPs:   []netip.Addr{netip.MustParseAddr("100.64.0.2")},
				ExitNodeOption: true,
			},
		},
	}
}

func TestSetExitNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arg      string
		wantAuto ipn.ExitNodeExpression
		wantIP   string
		wantErr  bool
	}{
		{name: "empty", arg: ""},
		{name: "off", arg: "off"},
		{name: "auto", arg: "auto", wantAuto: "any"},
		{name: "auto any", arg: "auto:any", wantAuto: "any"},
		{name: "auto expression", arg: "auto:geo:us", wantAuto: "geo:us"},
		{name: "peer short name", arg: "gateway", wantIP: "100.64.0.2"},
		{name: "peer fqdn", arg: "gateway.tailnet.ts.net", wantIP: "100.64.0.2"},
		{name: "tailscale ip", arg: "100.64.0.2", wantIP: "100.64.0.2"},
		{name: "unknown peer", arg: "nosuchpeer", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mp ipn.MaskedPrefs
			err := setExitNode(&mp, tt.arg, testStatus())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("setExitNode(%q) = nil, want error", tt.arg)
				}
				return
			}
			if err != nil {
				t.Fatalf("setExitNode(%q): %v", tt.arg, err)
			}
			if !mp.ExitNodeIDSet || !mp.ExitNodeIPSet || !mp.AutoExitNodeSet {
				t.Errorf("setExitNode(%q) left a mask bit unset: %+v", tt.arg, mp)
			}
			if got := mp.Prefs.AutoExitNode; got != tt.wantAuto {
				t.Errorf("setExitNode(%q) auto = %q, want %q", tt.arg, got, tt.wantAuto)
			}
			var gotIP string
			if mp.Prefs.ExitNodeIP.IsValid() {
				gotIP = mp.Prefs.ExitNodeIP.String()
			}
			if gotIP != tt.wantIP {
				t.Errorf("setExitNode(%q) ip = %q, want %q", tt.arg, gotIP, tt.wantIP)
			}
		})
	}
}

func TestDefaultStateDir(t *testing.T) {
	t.Parallel()

	dir, err := DefaultStateDir("ts-proxy")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("tailscale-socks", "ts-proxy"); !strings.HasSuffix(dir, want) {
		t.Errorf("DefaultStateDir = %q, want it to end in %q", dir, want)
	}
	other, err := DefaultStateDir("other")
	if err != nil {
		t.Fatal(err)
	}
	if other == dir {
		t.Error("different hostnames must not share a state directory")
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
		t.Run("rejects "+bad, func(t *testing.T) {
			t.Parallel()

			if _, err := DefaultStateDir(bad); err == nil {
				t.Errorf("DefaultStateDir(%q) = nil error, want error", bad)
			}
		})
	}
}
