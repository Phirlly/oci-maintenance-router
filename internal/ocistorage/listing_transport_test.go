package ocistorage

import (
	"crypto/x509"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestListingDoesNotUseProxyReuseConnectionsOrShareResults(t *testing.T) {
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
		_, _ = w.Write(pageJSON([]string{listedName(1)}, ""))
	})
	s := noticeStore(t, c, noticeState(t))
	var wg sync.WaitGroup
	ctx := budget(t)
	for range 8 {
		wg.Go(func() {
			p, err := s.ListPage(ctx, "")
			if err != nil || len(p.Names) != 1 || p.Names[0] != listedName(1) {
				t.Error("page corrupted", err)
				return
			}
			p.Names[0] = "caller change"
		})
	}
	wg.Wait()
	p, err := s.ListPage(ctx, "")
	if err != nil || p.Names[0] != listedName(1) {
		t.Fatal("mutation leaked", err)
	}
}

func TestListingTLSAndDroppedResponsesDoNotRetry(t *testing.T) {
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
		p, err := noticeStore(t, c, noticeState(t)).ListPage(budget(t), "")
		wantError(t, err, Transport, 0, false)
		emptyPage(t, p)
		want := int32(1)
		if untrusted {
			want = 0
		}
		if calls.Load() != want {
			t.Fatal("TLS bypass or retry")
		}
	}
}
