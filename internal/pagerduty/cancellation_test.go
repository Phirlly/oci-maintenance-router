package pagerduty

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestCancellationStopsDialAndTLS(t *testing.T) {
	for _, phase := range []string{"dial", "tls"} {
		t.Run(phase, func(t *testing.T) {
			c, _ := tlsClient(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
			entered, stopped := make(chan struct{}), make(chan struct{})
			if phase == "dial" {
				c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
					close(entered)
					<-ctx.Done()
					close(stopped)
					return nil, ctx.Err()
				}
			} else {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				c.endpoint = "https://" + listener.Addr().String() + "/v2/enqueue"
				c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					if addr != listener.Addr().String() {
						t.Error("external dial")
						return nil, errors.New("external dial")
					}
					return (&net.Dialer{}).DialContext(ctx, network, addr)
				}
				go func() {
					defer close(stopped)
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					defer conn.Close()
					close(entered)
					_, _ = io.Copy(io.Discard, conn)
				}()
			}
			ctx, cancel := context.WithCancel(budget(t))
			defer cancel()
			done := make(chan outcome, 1)
			go func() { r, e := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey)); done <- outcome{r, e} }()
			await(t, entered)
			cancel()
			got := receive(t, done)
			wantError(t, got.response, got.err, Canceled, true)
			if !errors.Is(got.err, context.Canceled) {
				t.Fatal("cancellation identity lost")
			}
			await(t, stopped)
		})
	}
}

func TestCancellationWhileWaitingForHeaders(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(stopped)
		// Returning would let net/http emit an implicit 200 during cancellation.
		<-release
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan outcome, 1)
	go func() { r, e := c.Send(ctx, []byte(savedEvent), []byte(syntheticKey)); done <- outcome{r, e} }()
	await(t, entered)
	cancel()
	got := receive(t, done)
	wantError(t, got.response, got.err, Canceled, true)
	await(t, stopped)
}
