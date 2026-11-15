package ocinotifications

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishNeverRetriesStatusesOrInventsAcceptance(t *testing.T) {
	key := generatedKey(t)
	for _, status := range []int{201, 204, 206, 301, 302, 303, 304, 307, 308, 400, 401, 403, 404, 429, 500, 503} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				serveChannel(w, r)
				return
			}
			calls.Add(1)
			w.Header().Set("Location", "http://%")
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
			io.WriteString(w, "private service response")
		})
		ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
		if err != nil {
			t.Fatal(err)
		}
		result, err := c.Publish(budget(t), ch, []byte(savedPayload))
		wantError(t, err, Service, status, true)
		if result != (Publication{Status: status, MayHaveSent: true}) || calls.Load() != 1 {
			t.Fatal("retried, followed redirect or fabricated acceptance")
		}
	}
}

func TestPublishRequiresStrictBoundedMessageID(t *testing.T) {
	for _, status := range []int{200, 202} {
		key := generatedKey(t)
		for _, body := range []string{
			"", "{", "null", "[]", "{}", `{"messageId":"x"} {}`, `{"MessageId":"x"}`, `{"messageId":null}`,
			`{"messageId":""}`, `{"messageId":"a b"}`, `{"messageId":"a\nb"}`,
			`{"messageId":"x","messageId":"y"}`, `{"messageId":"x","MessageId":"y"}`,
			`{"messageId":"x","TimeStamp":0}`, `{"messageId":"x","extra":{"x":1,"x":2}}`,
			`{"messageId":"x","extra":"\ud800"}`,
			`{"messageId":"` + strings.Repeat("a", 257) + `"}`,
		} {
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					serveChannel(w, r)
					return
				}
				w.WriteHeader(status)
				io.WriteString(w, body)
			})
			ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Publish(budget(t), ch, []byte(savedPayload))
			wantError(t, err, InvalidResponse, status, true)
			if result != (Publication{Status: status, MayHaveSent: true}) {
				t.Fatal("malformed acceptance fabricated")
			}
		}
	}

}

func TestPublishPreservesStatusThroughBodyFailure(t *testing.T) {
	for _, status := range []int{200, 202} {
		key := generatedKey(t)
		for _, phase := range []string{"range", "encoding", "oversized", "truncated", "stalled"} {
			cleanup := make(chan struct{})
			defer close(cleanup)
			c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					serveChannel(w, r)
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				switch phase {
				case "range":
					w.Header().Set("Content-Range", "bytes 0-1/2")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "oversized":
					w.Header().Set("Content-Length", "999999")
				case "truncated":
					w.Header().Set("Content-Length", "999")
				}
				w.WriteHeader(status)
				io.WriteString(w, `{"messageId":`)
				if phase == "stalled" {
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
					case <-cleanup:
					}
				}
			})
			ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(budget(t), 250*time.Millisecond)
			result, err := c.Publish(ctx, ch, []byte(savedPayload))
			cancel()
			code := InvalidResponse
			if phase == "oversized" {
				code = TooLarge
			}
			if phase == "stalled" {
				code = DeadlineExceeded
			}
			wantError(t, err, code, status, true)
			if result != (Publication{Status: status, MayHaveSent: true}) {
				t.Fatal("observed status lost")
			}
		}
	}

}

func TestDroppedPublishConnectionRemainsUncertainWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serveChannel(w, r)
			return
		}
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	})
	ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Publish(budget(t), ch, []byte(savedPayload))
	wantError(t, err, Transport, 0, true)
	if r != (Publication{MayHaveSent: true}) || calls.Load() != 1 {
		t.Fatal("uncertain publication replayed")
	}
}
