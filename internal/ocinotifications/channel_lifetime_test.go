package ocinotifications

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelReadsShareBudgetAndCleanup(t *testing.T) {
	for _, duration := range []time.Duration{time.Minute, time.Second} {
		c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, serveChannel)
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		parent, _ := ctx.Deadline()
		var children []context.Context
		dial := c.transport.DialContext
		c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			children = append(children, ctx)
			return dial(ctx, network, addr)
		}
		_, err := c.GetChannel(ctx, topicID, subscriptionID)
		if err != nil {
			t.Fatal(err)
		}
		if len(children) != 2 {
			t.Fatal("missing stages")
		}
		a, _ := children[0].Deadline()
		b, _ := children[1].Deadline()
		if !a.Equal(b) || a.After(parent) || time.Until(a) > 10*time.Second {
			t.Fatal("operation reset/extended budget")
		}
		if duration == time.Second && !a.Equal(parent) {
			t.Fatal("shorter caller deadline changed")
		}
		for _, child := range children {
			if child.Err() != context.Canceled {
				t.Fatal("child context leaked")
			}
		}
	}
}

func TestSecondReadSigningCannotDispatchAfterCancellation(t *testing.T) {
	var signs, calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t), beforeKey: func() {
		if signs.Add(1) == 2 {
			close(entered)
			<-release
		}
	}}, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); serveChannel(w, r) })
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan retrievalResult, 1)
	go func() { ch, err := c.GetChannel(ctx, topicID, subscriptionID); done <- retrievalResult{ch, err} }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	await(t, entered)
	cancel()
	select {
	case <-done:
		t.Fatal("signer abandoned")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	released = true
	wantError(t, takeError(t, done), Canceled, 0, false)
	if calls.Load() != 1 {
		t.Fatal("late subscription request dispatched")
	}
}

func TestCanceledSubscriptionResponseReturnsNoPartialChannel(t *testing.T) {
	entered := make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/20181201/topics/"+topicID {
			serveChannel(w, r)
			return
		}
		w.Write([]byte("{"))
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan retrievalResult, 1)
	go func() { ch, err := c.GetChannel(ctx, topicID, subscriptionID); done <- retrievalResult{ch, err} }()
	await(t, entered)
	cancel()
	err := takeError(t, done)
	e := err.(*Error)
	wantError(t, err, Canceled, e.Status, false)
}
