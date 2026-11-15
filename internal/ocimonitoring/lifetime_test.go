package ocimonitoring

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCancellationDuringSigningPreventsDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	calls := 0
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t), beforeKey: cancel}, func(http.ResponseWriter, *http.Request) { calls++ })
	if err := c.Publish(ctx, Observation{At: time.Now(), CompletedScan: true}); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("late dispatch: %v calls=%d", err, calls)
	}
}

func TestSigningCopiesValuesBeforeCredentialAccess(t *testing.T) {
	issues := 2
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t), beforeKey: func() { issues = 900 }}, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !containsMetricValue(body, "ReviewIssues", 2) {
			t.Error("caller mutation changed signed snapshot")
		}
		io.WriteString(w, `{"failedMetricsCount":0}`)
	})
	if err := c.Publish(budget(t), Observation{At: time.Now(), ReviewIssues: &issues}); err != nil {
		t.Fatal(err)
	}
}

func TestRequestCancellationStopsBodyRead(t *testing.T) {
	headers := make(chan struct{})
	finish := make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(headers)
		<-finish
	})
	ctx, cancel := context.WithCancel(budget(t))
	done := make(chan error, 1)
	go func() { done <- c.Publish(ctx, Observation{At: time.Now(), CompletedScan: true}) }()
	await(t, headers)
	cancel()
	defer close(finish)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("body read outlived cancellation")
	}
}

func TestDialUsesOriginalCancellation(t *testing.T) {
	c, err := New("us-ashburn-1", tenancy, compartment, "installation-1", syntheticKey{key: generatedKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	c.transport.DialContext = func(work context.Context, _, _ string) (net.Conn, error) {
		cancel()
		if work.Err() == nil {
			t.Error("dial detached from caller")
		}
		return nil, work.Err()
	}
	if err := c.Publish(ctx, Observation{At: time.Now(), CompletedScan: true}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
