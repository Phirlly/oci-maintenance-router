package ocimysql

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestCancellationStopsConnectionSetup(t *testing.T) {
	key := generatedKey(t)
	for _, phase := range []string{"dial", "tls"} {
		t.Run(phase, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
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
				c.endpoint = "https://" + listener.Addr().String()
				c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					if addr != listener.Addr().String() {
						t.Error("unexpected external dial")
						return nil, errors.New("unexpected dial")
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
			done := make(chan retrievalResult, 1)
			go func() { b, err := c.Get(ctx, dbID); done <- retrievalResult{b, err} }()
			await(t, entered)
			cancel()
			err := takeError(t, done)
			wantError(t, err, Canceled, 0)
			if !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation identity")
			}
			await(t, stopped)
		})
	}
}

func TestCancellationStopsWhileAwaitingHeadersOrBody(t *testing.T) {
	key := generatedKey(t)
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			entered, stopped := make(chan struct{}), make(chan struct{})
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if phase == "body" {
					_, _ = io.WriteString(w, "{")
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
				close(stopped)
			})
			ctx, cancel := context.WithCancel(budget(t))
			defer cancel()
			done := make(chan retrievalResult, 1)
			go func() { b, err := c.Get(ctx, dbID); done <- retrievalResult{b, err} }()
			await(t, entered)
			cancel()
			err := takeError(t, done)
			var got *Error
			// A server flush alone does not prove the client received headers.
			// The separate stalled-body deadline test requires observed status200.
			if !errors.As(err, &got) || (got.Status != 0 && got.Status != 200) {
				t.Fatalf("unexpected cancellation: %v", err)
			}
			wantError(t, err, Canceled, got.Status)
			await(t, stopped)
		})
	}
}
