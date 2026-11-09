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

func TestListingCancellationStopsDialHeadersAndBody(t *testing.T) {
	key := generatedKey(t)
	for _, phase := range []string{"dial", "headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			entered, stopped := make(chan struct{}), make(chan struct{})
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if phase == "body" {
					w.WriteHeader(200)
					_, _ = io.WriteString(w, "{")
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
				close(stopped)
			})
			if phase == "dial" {
				c.transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
					close(entered)
					<-ctx.Done()
					close(stopped)
					return nil, ctx.Err()
				}
			}
			s := noticeStore(t, c, noticeState(t))
			ctx, cancel := context.WithCancel(budget(t))
			defer cancel()
			done := make(chan error, 1)
			go func() {
				p, err := s.ListPage(ctx, "")
				if p.Names != nil {
					t.Error("canceled page returned")
				}
				done <- err
			}()
			await(t, entered)
			cancel()
			err := takeError(t, done)
			var e *Error
			if !errors.As(err, &e) || (e.Status != 0 && e.Status != 200) || !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation", err)
			}
			wantError(t, err, Canceled, e.Status, false)
			await(t, stopped)
		})
	}
}

func TestListingEarlierDeadlineStopsBodyAndOtherReadsRemainIndependent(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "" {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "{")
			w.(http.Flusher).Flush()
			close(entered)
			<-r.Context().Done()
			close(stopped)
			return
		}
		_, _ = w.Write(pageJSON([]string{listedName(2)}, ""))
	})
	s := noticeStore(t, c, noticeState(t))
	ctx, cancel := context.WithTimeout(budget(t), 400*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.ListPage(ctx, ""); done <- err }()
	await(t, entered)
	if p, err := s.ListPage(budget(t), listedName(2)); err != nil || len(p.Names) != 1 {
		t.Fatal("independent read blocked", err)
	}
	err := takeError(t, done)
	wantError(t, err, DeadlineExceeded, 200, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline identity lost")
	}
	await(t, stopped)
	if _, err := s.ListPage(budget(t), listedName(2)); err != nil {
		t.Fatal("cancellation leaked", err)
	}
}
