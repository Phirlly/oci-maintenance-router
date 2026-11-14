package ocinotifications

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPublishOwnsSavedBytesBeforeDispatch(t *testing.T) {
	payload := []byte(savedPayload)
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serveChannel(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != savedPayload {
			t.Error("saved bytes changed")
		}
		io.WriteString(w, `{"messageId":"`+messageID+`"}`)
	})
	ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	dial := c.transport.DialContext
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		for i := range payload {
			payload[i] = 'x'
		}
		return dial(ctx, network, addr)
	}
	if r, err := c.Publish(budget(t), ch, payload); err != nil || r.MessageID != messageID {
		t.Fatal("owned publication failed")
	}
}

func TestConcurrentChannelsAndPublicationsRemainIndependent(t *testing.T) {
	var reads, publishes atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reads.Add(1)
			serveChannel(w, r)
			return
		}
		publishes.Add(1)
		io.WriteString(w, `{"messageId":"`+messageID+`"}`)
	})
	ctx := budget(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ch, err := c.GetChannel(ctx, topicID, subscriptionID)
			if err != nil {
				t.Error(err)
				return
			}
			snapshot := ch.Snapshot()
			snapshot.Endpoint = "caller mutation"
			if ch.Snapshot().Endpoint != "Team@example.invalid" {
				t.Error("shared target")
			}
			r, err := c.Publish(ctx, ch, []byte(savedPayload))
			if err != nil || r.MessageID != messageID {
				t.Error("concurrent publication failed")
			}
		})
	}
	wg.Wait()
	if reads.Load() != 16 || publishes.Load() != 8 {
		t.Fatal("lost or replayed operations")
	}
}
