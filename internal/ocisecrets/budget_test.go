package ocisecrets

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOperationCapsLongCallerDeadline(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var remaining time.Duration
	c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("missing operation deadline")
		}
		remaining = time.Until(deadline)
		return nil, errors.New("synthetic dial failure")
	}
	b, err := c.GetCurrent(ctx, testSecretID)
	wantError(t, err, Transport, 0)
	wantEmpty(t, b)
	if remaining <= 0 || remaining > 10*time.Second {
		t.Fatalf("operation budget=%s", remaining)
	}
}

func TestEarlierDeadlineStopsStalledBody(t *testing.T) {
	stopped := make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	b, err := c.GetCurrent(ctx, testSecretID)
	wantEmpty(t, b)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline identity lost: %v", err)
	}
	wantError(t, err, DeadlineExceeded, 200)
	await(t, stopped)
}

func TestExpiredContextNeverConsultsCredentials(t *testing.T) {
	c, err := New("us-example-1", "exm", unusedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	b, err := c.GetCurrent(ctx, testSecretID)
	wantEmpty(t, b)
	wantError(t, err, DeadlineExceeded, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline identity lost")
	}
}
