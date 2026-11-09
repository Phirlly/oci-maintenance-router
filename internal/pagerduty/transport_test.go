package pagerduty

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLostResponseDoesNotReplayOrExposeCredentials(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, Transport, true)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("ordinary connection loss mislabeled as cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("unknown outcome replayed")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		text := fmt.Sprintf(format, err)
		if strings.Contains(text, syntheticKey) || strings.Contains(text, "http") || strings.Contains(text, "dedup") {
			t.Fatal("private diagnostic detail exposed")
		}
	}
}

func TestUntrustedTLSAndOversizedHeaders(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-Private", strings.Repeat("x", 20<<10))
		w.WriteHeader(202)
	})
	got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, Transport, true)
	if calls.Load() != 1 {
		t.Fatal("header failure replayed")
	}
	c.transport.TLSClientConfig.RootCAs = x509.NewCertPool()
	got, err = c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, Transport, true)
	if calls.Load() != 1 {
		t.Fatal("request sent before TLS verification")
	}
}

func TestFreshHTTP1ConnectionsIgnoreAmbientProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	var mu sync.Mutex
	seen := map[string]bool{}
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.ProtoMajor != 1 || seen[r.RemoteAddr] {
			t.Error("connection reused/multiplexed")
		}
		seen[r.RemoteAddr] = true
		w.WriteHeader(202)
	})
	if c.transport.Proxy != nil {
		t.Fatal("proxy enabled")
	}
	for range 3 {
		got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
		if err != nil || got.Status != 202 {
			t.Fatal("fresh exchange failed")
		}
	}
}
