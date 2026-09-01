package proxy

import (
	"io"
	"net"
	"net/http"
	"strings"
)

// NewHTTPProxy returns a forward-proxy handler: CONNECT tunnels and plain
// absolute-URI requests are both sent over dial.
func NewHTTPProxy(dial DialFunc) http.Handler { return &httpProxy{dial: dial} }

type httpProxy struct {
	dial DialFunc
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
	transport := &http.Transport{DialContext: p.dial}
	defer transport.CloseIdleConnections()

	outreq := r.Clone(r.Context())
	outreq.RequestURI = ""
	for _, h := range hopByHopHeaders {
		outreq.Header.Del(h)
	}
	resp, err := transport.RoundTrip(outreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range hopByHopHeaders {
		resp.Header.Del(h)
	}
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

	client, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "hijack: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	go io.Copy(upstream, client)
	io.Copy(client, upstream)
}

var hopByHopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}
