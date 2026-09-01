package proxy

import (
	"context"
	"errors"
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
	t.Parallel()

	// The origin reports back the headers it received, so the test can tell
	// what the proxy forwarded.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Origin-Secret", r.Header.Get("X-Secret"))
		w.Header().Set("X-Origin-Proxy-Auth", r.Header.Get("Proxy-Authorization"))
		io.WriteString(w, "hello "+r.Host)
	}))
	defer origin.Close()
	originHost := strings.TrimPrefix(origin.URL, "http://")

	proxySrv := httptest.NewServer(NewHTTPProxy(fixedDial(originHost)))
	defer proxySrv.Close()

	proxyURL, err := url.Parse(proxySrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	t.Run("plain http", func(t *testing.T) {
		resp, err := client.Get("http://server.tailnet.ts.net/")
		if err != nil {
			t.Fatalf("plain http: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(body); got != "hello server.tailnet.ts.net" {
			t.Fatalf("plain http body = %q", got)
		}
	})

	t.Run("connect tunnel", func(t *testing.T) {
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

		// The tunnel carries raw bytes: speak HTTP/1.1 over it.
		io.WriteString(conn, "GET / HTTP/1.1\r\nHost: tunneled\r\nConnection: close\r\n\r\n")
		tunneled, err := io.ReadAll(conn)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(tunneled), "hello tunneled") {
			t.Fatalf("tunneled response = %q", tunneled)
		}
	})

	t.Run("hop-by-hop headers are not forwarded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://server.tailnet.ts.net/", nil)
		req.Header.Set("Connection", "X-Secret")
		req.Header.Set("X-Secret", "leak")
		req.Header.Set("Proxy-Authorization", "Basic zzz")

		rec := httptest.NewRecorder()
		NewHTTPProxy(fixedDial(originHost)).ServeHTTP(rec, req)

		if got := rec.Result().Header.Get("X-Origin-Secret"); got != "" {
			t.Errorf("origin saw X-Secret = %q, want it dropped (named by Connection)", got)
		}
		if got := rec.Result().Header.Get("X-Origin-Proxy-Auth"); got != "" {
			t.Errorf("origin saw Proxy-Authorization = %q, want it dropped", got)
		}
	})

	t.Run("relative URI is rejected", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewHTTPProxy(fixedDial(originHost)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/relative", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("a dial failure is a bad gateway", func(t *testing.T) {
		failing := func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("no route to tailnet")
		}
		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "http://server.tailnet.ts.net/", nil),
			httptest.NewRequest(http.MethodConnect, "http://server.tailnet.ts.net:443", nil),
		} {
			rec := httptest.NewRecorder()
			NewHTTPProxy(failing).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadGateway {
				t.Errorf("%s status = %d, want %d", req.Method, rec.Code, http.StatusBadGateway)
			}
		}
	})
}

func TestRemoveHopByHop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   http.Header
		gone []string
		kept []string
	}{
		{
			name: "fixed list",
			in:   http.Header{"Proxy-Authorization": {"Basic zzz"}, "Upgrade": {"websocket"}, "Accept": {"*/*"}},
			gone: []string{"Proxy-Authorization", "Upgrade"},
			kept: []string{"Accept"},
		},
		{
			name: "named by Connection",
			in:   http.Header{"Connection": {"X-A, X-B"}, "X-A": {"1"}, "X-B": {"2"}, "X-C": {"3"}},
			gone: []string{"Connection", "X-A", "X-B"},
			kept: []string{"X-C"},
		},
		{
			name: "several Connection values",
			in:   http.Header{"Connection": {"X-A", "close, X-B"}, "X-A": {"1"}, "X-B": {"2"}},
			gone: []string{"X-A", "X-B"},
		},
		{
			name: "nothing to remove",
			in:   http.Header{"Accept": {"*/*"}},
			kept: []string{"Accept"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			removeHopByHop(tt.in)
			for _, h := range tt.gone {
				if got := tt.in.Get(h); got != "" {
					t.Errorf("%s = %q, want it removed", h, got)
				}
			}
			for _, h := range tt.kept {
				if tt.in.Get(h) == "" {
					t.Errorf("%s was removed, want it kept", h)
				}
			}
		})
	}
}

func TestSOCKS5PassesNamesToDialer(t *testing.T) {
	t.Parallel()

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
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "hello peer.tailnet.ts.net" {
		t.Fatalf("body = %q", got)
	}
	if got := <-asked; got != "peer.tailnet.ts.net:80" {
		t.Fatalf("dialer asked for %q, want the unresolved name", got)
	}
}
