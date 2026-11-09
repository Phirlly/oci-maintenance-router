package pagerduty

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOperationCapsLongDeadlineAndPropagatesEarlierOne(t *testing.T) {
	for _, duration := range []time.Duration{time.Minute, time.Second} {
		c, _ := tlsClient(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		callerDeadline, _ := ctx.Deadline()
		var observed time.Time
		c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			observed, _ = ctx.Deadline()
			return nil, errors.New("private dial details")
		}
		before := time.Now()
		got, err := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey))
		cancel()
		wantError(t, got, err, Transport, true)
		if observed.IsZero() || observed.After(time.Now().Add(10*time.Second)) || observed.Before(before) {
			t.Fatal("invalid operation budget")
		}
		if duration == time.Second && !observed.Equal(callerDeadline) {
			t.Fatal("caller deadline extended")
		}
	}
}

func TestEarlierDeadlineStopsHeaderWait(t *testing.T) {
	stopped := make(chan struct{})
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		close(stopped)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	got, err := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey))
	wantError(t, got, err, DeadlineExceeded, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline identity lost")
	}
	await(t, stopped)
}
