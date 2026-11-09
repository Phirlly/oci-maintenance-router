package pagerduty

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
	c, _ := tlsClient(t, func(http.ResponseWriter, *http.Request) { t.Error("request unexpectedly written") })
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
	done := make(chan outcome, 1)
	go func() { r, e := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey)); done <- outcome{r, e} }()
	await(t, entered)
	cancel()
	got := receive(t, done)
	wantError(t, got.response, got.err, Canceled, true)
	await(t, closed)
}
