package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fixedDial stands in for the tailnet dialer: every address lands on target.
func fixedDial(target string) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return new(net.Dialer).DialContext(ctx, "tcp", target)
	}
}

func TestHTTPProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello "+r.Host)
	}))
	defer origin.Close()
	originHost := strings.TrimPrefix(origin.URL, "http://")

	proxySrv := httptest.NewServer(NewHTTPProxy(fixedDial(originHost)))
	defer proxySrv.Close()

	proxyURL, _ := url.Parse(proxySrv.URL)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	// Plain HTTP: absolute-URI request forwarded through the proxy.
	resp, err := client.Get("http://server.tailnet.ts.net/")
	if err != nil {
		t.Fatalf("plain http: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "hello server.tailnet.ts.net" {
		t.Fatalf("plain http body = %q", got)
	}

	// CONNECT: tunnel raw bytes, then speak HTTP/1.1 over it.
	conn, err := net.Dial("tcp", proxyURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "CONNECT server.tailnet.ts.net:443 HTTP/1.1\r\nHost: server.tailnet.ts.net:443\r\n\r\n")
	buf := make([]byte, len("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("connect read: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 200") {
		t.Fatalf("connect response = %q", buf)
	}
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: tunneled\r\nConnection: close\r\n\r\n")
	tunneled, _ := io.ReadAll(conn)
	if !strings.Contains(string(tunneled), "hello tunneled") {
		t.Fatalf("tunneled response = %q", tunneled)
	}
}

func TestSOCKS5PassesNamesToDialer(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello "+r.Host)
	}))
	defer origin.Close()

	// Record what the SOCKS5 server asks the dialer for: it must be the name,
	// not an IP resolved on this host.
	asked := make(chan string, 1)
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		select {
		case asked <- addr:
		default:
		}
		return fixedDial(strings.TrimPrefix(origin.URL, "http://"))(ctx, network, addr)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go ServeSOCKS5(ln, dial, discardLogger())

	client := &http.Client{Transport: &http.Transport{Proxy: mustProxyURL(t, "socks5h://"+ln.Addr().String())}}
	resp, err := client.Get("http://peer.tailnet.ts.net/")
	if err != nil {
		t.Fatalf("socks5 get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "hello peer.tailnet.ts.net" {
		t.Fatalf("body = %q", got)
	}
	if got := <-asked; got != "peer.tailnet.ts.net:80" {
		t.Fatalf("dialer asked for %q, want the unresolved name", got)
	}
}
