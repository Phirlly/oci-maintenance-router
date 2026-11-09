package ocisecrets

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
	for _, status := range []int{201, 204, 206, 301, 302, 307, 308, 400, 401, 403, 404, 429, 500, 503} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "/redirected")
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "synthetic-private-response")
		})
		b, err := c.GetCurrent(budget(t), testSecretID)
		wantError(t, err, Service, status)
		wantEmpty(t, b)
		if calls.Load() != 1 || err.Error() != "secret retrieval: service" {
			t.Fatal("service response retried or disclosed")
		}
	}
}

func TestUntrustedTLSNeverSendsCredentials(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	b, err := c.GetCurrent(budget(t), testSecretID)
	wantError(t, err, Transport, 0)
	wantEmpty(t, b)
	if calls.Load() != 0 || err.Error() != "secret retrieval: transport" {
		t.Fatal("TLS bypass or diagnostic leak")
	}
}

func TestFreshHTTP1ConnectionsIgnoreAmbientProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	var previous string
	var mu sync.Mutex
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.ProtoMajor != 1 || r.RemoteAddr == previous {
			t.Error("connection reused or multiplexed")
		}
		previous = r.RemoteAddr
		_, _ = w.Write(bundleBody(t, testSecretID, 7, []byte("synthetic-key")))
	})
	if c.transport.Proxy != nil {
		t.Fatal("ambient proxy enabled")
	}
	for range 3 {
		if _, err := c.GetCurrent(budget(t), testSecretID); err != nil {
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
		_ = conn.Close()
	})
	b, err := c.GetVersion(budget(t), testSecretID, 7)
	wantError(t, err, Transport, 0)
	wantEmpty(t, b)
	if calls.Load() != 1 {
		t.Fatal("dropped connection caused a retry")
	}
}
