package ociownership

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestComputeFailuresReturnNoMetadataAndNeverRetry(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{200, 302, 403, 404, 429, 503} {
		var calls atomic.Int32
		c, err := NewCompute("us-ashburn-1", "iad", tenancy, syntheticKey{key: key})
		if err != nil {
			t.Fatal(err)
		}
		c, _ = bindTLSClient(t, c, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "http://%")
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
			if status == 200 {
				w.(http.Flusher).Flush()
				io.WriteString(w, strings.Repeat(" ", bodyLimit+1))
			} else {
				io.WriteString(w, "synthetic-private-service-response")
			}
		})
		m, err := c.Get(budget(t), instanceID)
		code := Service
		if status == 200 {
			code = TooLarge
		}
		wantError(t, err, code, status)
		wantEmpty(t, m)
		if calls.Load() != 1 {
			t.Fatal("read was retried or redirected")
		}
	}
}

func TestComputeCancellationStopsTheRead(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, err := NewCompute("us-ashburn-1", "iad", tenancy, syntheticKey{key: generatedKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	c, _ = bindTLSClient(t, c, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(stopped)
		<-release // Keep implicit success headers behind the observed cancellation.
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan retrievalResult, 1)
	go func() { m, err := c.Get(ctx, instanceID); done <- retrievalResult{m, err} }()
	await(t, entered)
	cancel()
	wantError(t, takeError(t, done), Canceled, 0)
	await(t, stopped)
}
