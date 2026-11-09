package ociannouncements

import (
	"crypto/x509"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestServiceResponsesNeverRetryOrFollowRedirects(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{201, 204, 206, 301, 302, 303, 307, 308, 400, 401, 403, 404, 429, 500, 503} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "http://%")
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
			io.WriteString(w, "synthetic-private-response")
		})
		n, err := c.Get(budget(t), noticeID)
		wantError(t, err, Service, status)
		wantEmpty(t, n)
		p, err := c.ListPage(budget(t), "")
		wantError(t, err, Service, status)
		if len(p.IDs) != 0 || p.NextPage != "" || calls.Load() != 2 {
			t.Fatal("partial result, retry or redirect")
		}
	}
}

func TestUntrustedTLSNeverSendsCredentials(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	n, err := c.Get(budget(t), noticeID)
	wantError(t, err, Transport, 0)
	wantEmpty(t, n)
	if calls.Load() != 0 {
		t.Fatal("TLS verification bypassed")
	}
}

func TestFreshHTTP1ConnectionsIgnoreAmbientProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	addresses := map[string]bool{}
	var mu sync.Mutex
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.ProtoMajor != 1 || !r.Close || addresses[r.RemoteAddr] {
			t.Error("reused or multiplexed connection")
		}
		addresses[r.RemoteAddr] = true
		io.WriteString(w, minimal)
	})
	if c.transport.Proxy != nil {
		t.Fatal("ambient proxy enabled")
	}
	for range 3 {
		if _, err := c.Get(budget(t), noticeID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDroppedConnectionDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	})
	n, err := c.Get(budget(t), noticeID)
	wantError(t, err, Transport, 0)
	wantEmpty(t, n)
	if calls.Load() != 1 {
		t.Fatal("transport replayed read")
	}
}
