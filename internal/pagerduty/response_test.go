package pagerduty

import (
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestObservedStatusesDoNotRetryOrRedirect(t *testing.T) {
	for _, status := range []int{200, 201, 202, 204, 301, 302, 307, 308, 400, 401, 403, 429, 500, 503, 599, 600, 999} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirected")
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private service text")
			})
			before := time.Now().UTC()
			got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
			if err != nil || got.Status != status || calls.Load() != 1 {
				t.Fatalf("status/retry failure: %+v %v", got, err)
			}
			if status == 429 || (status >= 500 && status <= 599) {
				if got.RetryAfter.Before(before.Add(120*time.Second)) || got.RetryAfter.After(time.Now().Add(120*time.Second)) {
					t.Fatal("retry delay not based on receipt")
				}
			} else if !got.RetryAfter.IsZero() {
				t.Fatal("retry hint on nonretryable status")
			}
		})
	}
}

func TestAcceptedResponseDoesNotWaitForBody(t *testing.T) {
	for _, framing := range []string{"length", "chunked"} {
		t.Run(framing, func(t *testing.T) {
			stopped := make(chan struct{})
			c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if framing == "length" {
					w.Header().Set("Content-Length", "999999999")
				}
				w.WriteHeader(202)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(stopped)
			})
			got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
			if err != nil || got.Status != 202 {
				t.Fatalf("accepted status lost: %+v %v", got, err)
			}
			await(t, stopped)
		})
	}
}

func TestMalformedRedirectLocationPreservesObservedStatus(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		var calls atomic.Int32
		c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", ":invalid")
			w.WriteHeader(status)
		})
		got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
		if err != nil || got.Status != status || calls.Load() != 1 {
			t.Fatalf("observed redirect lost: %+v %v", got, err)
		}
	}
}
