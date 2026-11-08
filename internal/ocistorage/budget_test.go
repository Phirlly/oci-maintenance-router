package ocistorage

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperationCapsLongCallerDeadline(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	start := time.Now()
	var calls atomic.Int32
	c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(start.Add(9*time.Second)) || deadline.After(start.Add(10100*time.Millisecond)) {
			t.Error("operation did not enforce ten-second cap")
		}
		return nil, errors.New("synthetic-private-network-detail")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	_, err := c.Get(ctx, "state")
	wantError(t, err, Transport, 0, false)
	if calls.Load() != 1 || err.Error() != "object storage: transport" || errors.Unwrap(err) != nil {
		t.Fatal("network error leaked or retried")
	}
}

func TestAmbientProxyCannotRedirectSignedRequest(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("ETag", "tag") })
	if c.transport.Proxy != nil {
		t.Fatal("ambient proxy enabled")
	}
	if _, err := c.Get(budget(t), "state"); err != nil {
		t.Fatal(err)
	}
}
