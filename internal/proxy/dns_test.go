package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

// dnsCall records what the server asked the backend for.
type dnsCall struct {
	query  string
	family string
	from   netip.AddrPort
}

// fakeDNS stands in for *tsnode.Node: no tailnet, canned answers.
type fakeDNS struct {
	answer func(query []byte) ([]byte, error)
	calls  chan dnsCall
	srcs   chan netip.AddrPort
}

var _ DNSBackend = (*fakeDNS)(nil)

func (f *fakeDNS) DNSQuery(ctx context.Context, query []byte, family string, from netip.AddrPort) ([]byte, error) {
	f.calls <- dnsCall{query: string(query), family: family, from: from}
	return f.answer(query)
}

func (f *fakeDNS) HandleDNSTCPConn(conn net.Conn, src netip.AddrPort) {
	defer conn.Close()
	f.srcs <- src
	io.WriteString(conn, "pong")
}

func TestServeDNSUDP(t *testing.T) {
	t.Parallel()

	backend := &fakeDNS{
		calls: make(chan dnsCall, 4),
		answer: func(q []byte) ([]byte, error) {
			if string(q) == "boom" {
				return nil, errors.New("resolver down")
			}
			return append([]byte("answer:"), q...), nil
		},
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- ServeDNSUDP(context.Background(), pc, backend, discardLogger()) }()

	client, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ask := func(t *testing.T, q string) {
		t.Helper()
		if _, err := client.Write([]byte(q)); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T) string {
		t.Helper()
		buf := make([]byte, maxDNSMessage)
		n, err := client.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		return string(buf[:n])
	}

	t.Run("answers a query", func(t *testing.T) {
		ask(t, "hello")
		if got := read(t); got != "answer:hello" {
			t.Fatalf("reply = %q, want %q", got, "answer:hello")
		}
		call := <-backend.calls
		if call.query != "hello" || call.family != "udp" {
			t.Errorf("backend saw %+v, want query hello over udp", call)
		}
		if want := client.LocalAddr().String(); call.from.String() != want {
			t.Errorf("backend saw source %v, want %v", call.from, want)
		}
	})

	t.Run("a failing query leaves the server serving", func(t *testing.T) {
		ask(t, "boom")
		if call := <-backend.calls; call.query != "boom" {
			t.Fatalf("backend saw %q, want boom", call.query)
		}
		// No reply for "boom": the next read must return the later answer.
		ask(t, "again")
		if got := read(t); got != "answer:again" {
			t.Fatalf("reply = %q, want %q", got, "answer:again")
		}
	})

	pc.Close()
	if err := <-errc; !errors.Is(err, net.ErrClosed) {
		t.Errorf("ServeDNSUDP returned %v, want %v", err, net.ErrClosed)
	}
}

func TestServeDNSTCP(t *testing.T) {
	t.Parallel()

	backend := &fakeDNS{srcs: make(chan netip.AddrPort, 1)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { errc <- ServeDNSTCP(ln, backend) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pong" {
		t.Errorf("read %q, want the backend's answer", got)
	}
	if src := <-backend.srcs; src.String() != conn.LocalAddr().String() {
		t.Errorf("backend saw source %v, want %v", src, conn.LocalAddr())
	}

	ln.Close()
	if err := <-errc; !errors.Is(err, net.ErrClosed) {
		t.Errorf("ServeDNSTCP returned %v, want %v", err, net.ErrClosed)
	}
}
