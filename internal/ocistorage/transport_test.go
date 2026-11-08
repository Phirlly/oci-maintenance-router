package ocistorage

import (
	"crypto/x509"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResponsesNeverTriggerAutomaticAttempts(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{301, 302, 307, 308, 401, 429, 500, 503, 412, 201} {
		for _, write := range []bool{false, true} {
			var sends atomic.Int32
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				w.Header().Set("Location", "/redirected")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "synthetic-private-response")
			})
			var err error
			if write {
				_, err = c.Create(budget(t), "state", nil)
			} else {
				_, err = c.Get(budget(t), "state")
			}
			code, uncertain := Service, write
			if status == 412 && write {
				code, uncertain = Conflict, false
			}
			wantError(t, err, code, status, uncertain)
			if sends.Load() != 1 || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatalf("unsafe response handling: %v sends%d", err, sends.Load())
			}
		}
	}
}

func TestUntrustedTLSNeverReachesHandler(t *testing.T) {
	var sends atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { sends.Add(1) })
	c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	_, err := c.Create(budget(t), "state", nil)
	wantError(t, err, Transport, 0, true)
	if sends.Load() != 0 || err.Error() != "object storage: transport" {
		t.Fatal("TLS bypass or diagnostic leak")
	}
}

func TestEveryOperationUsesFreshHTTP1Connection(t *testing.T) {
	var previous string
	var mu sync.Mutex
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.ProtoMajor != 1 || r.RemoteAddr == previous {
			t.Error("reused or multiplexed connection")
		}
		previous = r.RemoteAddr
		w.Header().Set("ETag", "tag")
	})
	for range 3 {
		if _, err := c.Get(budget(t), "state"); err != nil {
			t.Fatal(err)
		}
	}
}
