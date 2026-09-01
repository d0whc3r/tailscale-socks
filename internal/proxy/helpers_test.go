package proxy

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"
)

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func mustProxyURL(t *testing.T, raw string) func(*http.Request) (*url.URL, error) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return http.ProxyURL(u)
}
