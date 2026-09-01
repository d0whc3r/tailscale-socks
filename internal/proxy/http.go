package proxy

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// idleConnTimeout matches net/http's own default. Without it the zero-value
// Transport keeps every pooled tailnet connection open for the life of the
// process.
const idleConnTimeout = 90 * time.Second

// NewHTTPProxy returns a forward-proxy handler: CONNECT tunnels and plain
// absolute-URI requests are both sent over dial.
func NewHTTPProxy(dial DialFunc) http.Handler {
	return &httpProxy{
		dial: dial,
		transport: &http.Transport{
			DialContext:     dial,
			IdleConnTimeout: idleConnTimeout,
		},
	}
}

type httpProxy struct {
	dial DialFunc
	// transport is shared by every request so tailnet connections are pooled.
	transport *http.Transport
}

func (p *httpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "this is a proxy, use an absolute URI", http.StatusBadRequest)
		return
	}

	outreq := r.Clone(r.Context())
	outreq.RequestURI = ""
	removeHopByHop(outreq.Header)

	resp, err := p.transport.RoundTrip(outreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	removeHopByHop(resp.Header)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *httpProxy) connect(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Host
	if addr == "" {
		addr = r.Host
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(strings.Trim(addr, "[]"), "443")
	}
	upstream, err := p.dial(r.Context(), "tcp", addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	client, brw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "hijack: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	// Read the client through brw, not the raw connection: a client that
	// pipelines its first payload behind the CONNECT request — a TLS
	// ClientHello in the same segment, say — leaves those bytes buffered in
	// the server's reader, and the raw connection no longer has them.
	//
	// Both copies end when either side closes: the deferred Close on the way
	// out unblocks the goroutine.
	go io.Copy(upstream, brw.Reader)
	io.Copy(client, upstream)
}

// removeHopByHop deletes the headers that must not be forwarded: the fixed
// list plus whatever the Connection header names (RFC 7230 section 6.1).
func removeHopByHop(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, name := range strings.Split(v, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}

var hopByHopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}
