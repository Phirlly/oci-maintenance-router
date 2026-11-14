package ocistorage

import (
	"crypto/x509"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestHistoryTLSAndDroppedResponsesNeverRetry(t *testing.T) {
	key := generatedKey(t)
	for _, untrusted := range []bool{false, true} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
		})
		if untrusted {
			c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
		}
		empty, err := c.HistoryIsEmpty(budget(t), "control/head.json")
		wantError(t, err, Transport, 0, false)
		want := int32(1)
		if untrusted {
			want = 0
		}
		if empty || calls.Load() != want {
			t.Fatal("TLS bypass, retry or false absence")
		}
	}
}

func TestHistoryConcurrentCallsUseFreshConnectionsWithoutProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	var mu sync.Mutex
	seen := map[string]bool{}
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if seen[r.RemoteAddr] || r.ProtoMajor != 1 {
			t.Error("connection reused")
		}
		seen[r.RemoteAddr] = true
		_, _ = io.WriteString(w, `{"items":[]}`)
	})
	var wg sync.WaitGroup
	ctx := budget(t)
	for range 8 {
		wg.Go(func() {
			empty, err := c.HistoryIsEmpty(ctx, "control/head.json")
			if err != nil || !empty {
				t.Error("concurrent history read failed", err)
			}
		})
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 8 {
		t.Fatal("history operations shared a connection")
	}
}
