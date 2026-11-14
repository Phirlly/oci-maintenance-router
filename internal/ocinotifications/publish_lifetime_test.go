package ocinotifications

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type publishOutcome struct {
	result Publication
	err    error
}

func receivePublish(t *testing.T, done <-chan publishOutcome) publishOutcome {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("publication did not stop")
		return publishOutcome{}
	}
}
func TestPublishSigningCancellationNeverDispatches(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); serveChannel(w, r) })
	ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	c.signer = common.DefaultRequestSigner(syntheticKey{key: generatedKey(t), beforeKey: func() { close(entered); <-release }})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan publishOutcome, 1)
	go func() { r, e := c.Publish(ctx, ch, []byte(savedPayload)); done <- publishOutcome{r, e} }()
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
	r := receivePublish(t, done)
	wantError(t, r.err, Canceled, 0, false)
	if r.result != (Publication{}) || calls.Load() != 2 {
		t.Fatal("late publication dispatched")
	}
}

func TestPublishCancellationClosesDispatchAndKeepsUncertainty(t *testing.T) {
	key := generatedKey(t)
	for _, stage := range []string{"dial", "headers", "body"} {
		entered, stopped := make(chan struct{}), make(chan struct{})
		cleanup := make(chan struct{})
		defer close(cleanup)
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				serveChannel(w, r)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			if stage == "body" {
				w.Write([]byte("{"))
				w.(http.Flusher).Flush()
			}
			close(entered)
			select {
			case <-r.Context().Done():
			case <-cleanup:
			}
			close(stopped)
		})
		ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
		if err != nil {
			t.Fatal(err)
		}
		if stage == "dial" {
			c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				close(entered)
				<-ctx.Done()
				close(stopped)
				return nil, ctx.Err()
			}
		}
		ctx, cancel := context.WithCancel(budget(t))
		done := make(chan publishOutcome, 1)
		go func() { r, e := c.Publish(ctx, ch, []byte(savedPayload)); done <- publishOutcome{r, e} }()
		await(t, entered)
		cancel()
		r := receivePublish(t, done)
		wantError(t, r.err, Canceled, r.result.Status, true)
		if r.result.MessageID != "" || !r.result.MayHaveSent || (r.result.Status != 0 && r.result.Status != 200) {
			t.Fatal("canceled publication claimed acceptance")
		}
		await(t, stopped)
	}
}
