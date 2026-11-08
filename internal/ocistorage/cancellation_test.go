package ocistorage

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCancellationStopsDialAndTLS(t *testing.T) {
	key := generatedKey(t)
	for _, phase := range []string{"dial", "tls"} {
		t.Run(phase, func(t *testing.T) {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
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
						t.Error("unexpected dial")
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
			done := make(chan error, 1)
			go func() { _, err := c.Get(ctx, "state"); done <- err }()
			await(t, entered)
			cancel()
			err := takeError(t, done)
			wantError(t, err, Canceled, 0, false)
			if !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation identity")
			}
			await(t, stopped)
		})
	}
}

func TestCancellationCoversHeadersAndResponseBody(t *testing.T) {
	key := generatedKey(t)
	for _, phase := range []string{"headers", "get_body", "put_body"} {
		t.Run(phase, func(t *testing.T) {
			entered, stopped := make(chan struct{}), make(chan struct{})
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if phase != "headers" {
					w.Header().Set("ETag", "tag")
					w.WriteHeader(200)
					_, _ = io.WriteString(w, "x")
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
				close(stopped)
			})
			ctx, cancel := context.WithCancel(budget(t))
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				if phase == "put_body" {
					_, err = c.Create(ctx, "state", nil)
				} else {
					_, err = c.Get(ctx, "state")
				}
				done <- err
			}()
			await(t, entered)
			cancel()
			err := takeError(t, done)
			var got *Error
			// The server flushing does not prove the client received its headers
			// before cancellation. Both observed statuses are legitimate here.
			if !errors.As(err, &got) || (got.Status != 0 && got.Status != 200) {
				t.Fatalf("unexpected cancellation: %v", err)
			}
			wantError(t, err, Canceled, got.Status, phase == "put_body")
			await(t, stopped)
		})
	}
}

func TestConcurrentRequestsDoNotShareCancellation(t *testing.T) {
	entered := make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/n/example/b/ledger/o/blocked" {
			close(entered)
			<-r.Context().Done()
			return
		}
		w.Header().Set("ETag", "tag")
		_, _ = io.WriteString(w, "independent")
	})
	ctx, cancel := context.WithCancel(budget(t))
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Get(ctx, "blocked"); done <- err }()
	await(t, entered)
	if obj, err := c.Get(budget(t), "second"); err != nil || string(obj.Body) != "independent" {
		t.Fatal("concurrent request blocked")
	}
	cancel()
	wantError(t, takeError(t, done), Canceled, 0, false)
	if _, err := c.Get(budget(t), "third"); err != nil {
		t.Fatal("cancellation leaked")
	}
}

func TestEarlierDeadlineStopsStalledRead(t *testing.T) {
	for _, write := range []bool{false, true} {
		stopped := make(chan struct{})
		c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", "tag")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(stopped)
		})
		ctx, cancel := context.WithTimeout(budget(t), 250*time.Millisecond)
		defer cancel()
		var err error
		if write {
			_, err = c.Create(ctx, "state", nil)
		} else {
			_, err = c.Get(ctx, "state")
		}
		wantError(t, err, DeadlineExceeded, 200, write)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("lost deadline identity")
		}
		await(t, stopped)
	}
}

func takeError(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not stop")
		return nil
	}
}
