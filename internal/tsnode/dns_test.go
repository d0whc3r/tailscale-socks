package tsnode

import (
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestParseAddrs(t *testing.T) {
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
	msg := dnsmessage.Message{Header: dnsmessage.Header{Response: true, RCode: dnsmessage.RCodeNameError}}
	packed, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAddrs(packed); err == nil {
		t.Fatal("parseAddrs on NXDOMAIN = nil error, want error")
	}
}
