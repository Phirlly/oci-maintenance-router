package pagerduty

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSendUsesExactEnvelopeAndSingleRequest(t *testing.T) {
	var calls atomic.Int32
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.RequestURI() != "/v2/enqueue" || r.ProtoMajor != 1 || !r.Close || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept-Encoding") != "" || r.Header.Get("Authorization") != "" {
			t.Error("wrong wire contract")
		}
		body, _ := io.ReadAll(r.Body)
		expected := "{\"routing_key\":\"" + syntheticKey + "\"," + savedEvent[1:]
		if string(body) != expected || r.ContentLength != int64(len(expected)) {
			t.Error("saved payload changed")
		}
		w.WriteHeader(202)
	})
	got, err := c.Send(budget(t), []byte(savedEvent), []byte(syntheticKey))
	if err != nil || got.Status != 202 || !got.RetryAfter.IsZero() || got.RetryAfterOverflow || calls.Load() != 1 {
		t.Fatalf("send failed: %+v %v", got, err)
	}
}

func TestSendOwnsBytesBeforeDial(t *testing.T) {
	original := []byte(savedEvent)
	key := []byte(syntheticKey)
	c, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(syntheticKey)) || !strings.HasSuffix(string(body), savedEvent[1:]) {
			t.Error("caller mutation changed body")
		}
		w.WriteHeader(202)
	})
	dial := c.transport.DialContext
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		for i := range original {
			original[i] = 'x'
		}
		for i := range key {
			key[i] = 'x'
		}
		return dial(ctx, network, addr)
	}
	got, err := c.Send(budget(t), original, key)
	if err != nil || got.Status != 202 {
		t.Fatalf("send: %v", err)
	}
}
