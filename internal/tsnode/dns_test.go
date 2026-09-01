package tsnode

import (
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestParseAddrs(t *testing.T) {
	t.Parallel()

	name := dnsmessage.MustNewName("peer.tailnet.ts.net.")
	hdr := dnsmessage.ResourceHeader{Name: name, Class: dnsmessage.ClassINET}
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{Response: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
		Answers: []dnsmessage.Resource{
			{Header: hdr, Body: &dnsmessage.CNAMEResource{CNAME: name}}, // must be skipped
			{Header: hdr, Body: &dnsmessage.AResource{A: [4]byte{100, 64, 0, 2}}},
			{Header: hdr, Body: &dnsmessage.AAAAResource{AAAA: netip.MustParseAddr("fd7a::1").As16()}},
		},
	}
	packed, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseAddrs(packed)
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Addr{netip.MustParseAddr("100.64.0.2"), netip.MustParseAddr("fd7a::1")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestParseAddrsRCodeError(t *testing.T) {
	t.Parallel()

	msg := dnsmessage.Message{Header: dnsmessage.Header{Response: true, RCode: dnsmessage.RCodeNameError}}
	packed, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAddrs(packed); err == nil {
		t.Fatal("parseAddrs on NXDOMAIN = nil error, want error")
	}
}

func TestDNSName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "relative", host: "peer", want: "peer."},
		{name: "already qualified", host: "peer.tailnet.ts.net.", want: "peer.tailnet.ts.net."},
		{name: "empty", host: "", want: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := dnsname(tt.host); got != tt.want {
				t.Errorf("dnsname(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

// FuzzParseAddrs feeds parseAddrs arbitrary bytes: the responses it parses come
// off the network, so it must never panic and never return an invalid address.
func FuzzParseAddrs(f *testing.F) {
	name := dnsmessage.MustNewName("peer.tailnet.ts.net.")
	answer := dnsmessage.Message{
		Header:    dnsmessage.Header{Response: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: name, Class: dnsmessage.ClassINET},
			Body:   &dnsmessage.AResource{A: [4]byte{100, 64, 0, 2}},
		}},
	}
	for _, msg := range []dnsmessage.Message{
		answer,
		{Header: dnsmessage.Header{Response: true}},
		{Header: dnsmessage.Header{Response: true, RCode: dnsmessage.RCodeNameError}},
	} {
		packed, err := msg.Pack()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(packed)
	}
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, resp []byte) {
		addrs, err := parseAddrs(resp)
		if err != nil {
			return
		}
		for _, a := range addrs {
			if !a.IsValid() {
				t.Fatalf("parseAddrs returned an invalid address in %v", addrs)
			}
		}
	})
}
