package ocinotifications

import (
	"context"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
)

type stalledWriteConn struct {
	net.Conn
	block                *atomic.Bool
	entered, closed      chan struct{}
	enterOnce, closeOnce sync.Once
}

func (c *stalledWriteConn) Write(p []byte) (int, error) {
	if c.block.Load() {
		c.enterOnce.Do(func() { close(c.entered) })
		<-c.closed
		return 0, net.ErrClosed
	}
	return c.Conn.Write(p)
}
func (c *stalledWriteConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestCancellationClosesStalledRequestWrite(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serveChannel(w, r)
			return
		}
		t.Error("publication unexpectedly written")
	})
	channel, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	var block atomic.Bool
	entered, closed := make(chan struct{}), make(chan struct{})
	dial := c.transport.DialContext
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &stalledWriteConn{Conn: conn, block: &block, entered: entered, closed: closed}, nil
	}
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteHeaders: func() { block.Store(true) }})
	done := make(chan publishOutcome, 1)
	go func() { r, e := c.Publish(ctx, channel, []byte(savedPayload)); done <- publishOutcome{r, e} }()
	await(t, entered)
	cancel()
	got := receivePublish(t, done)
	wantError(t, got.err, Canceled, 0, true)
	if got.result != (Publication{MayHaveSent: true}) {
		t.Fatal("stalled write claimed acceptance")
	}
	await(t, closed)
}
