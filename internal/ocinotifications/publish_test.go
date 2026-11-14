package ocinotifications

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const savedPayload = `{"title":"OCI maintenance router operator test","body":"Confirm this operator notification. OMR-OP-0123456789abcdef0123456789abcdef"}`
const messageID = "d85ba735-f27e-4a6a-a8e1-6244b0b1634d"

func TestPublishPreservesSavedBytesAndRequiresAcceptedResult(t *testing.T) {
	key := generatedKey(t)
	var publications atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serveChannel(w, r)
			return
		}
		publications.Add(1)
		body, _ := io.ReadAll(r.Body)
		verifySignature(t, r, key, body)
		if r.Method != "POST" || r.Host != strings.TrimPrefix(dataOrigin, "https://") || r.URL.Path != "/20181201/topics/"+topicID+"/messages" || r.URL.RawQuery != "" || string(body) != savedPayload || r.ContentLength != int64(len(body)) || r.Header.Get("Content-Type") != "application/json" || !r.Close || r.ProtoMajor != 1 {
			t.Error("wrong saved publication")
		}
		for _, h := range []string{"opc-retry-token", "Idempotency-Key", "X-Idempotency-Key", "If-Match"} {
			if r.Header.Get(h) != "" {
				t.Error("unexpected retry/condition header")
			}
		}
		io.WriteString(w, `{"messageId":"`+messageID+`","timeStamp":"ignored","future":true}`)
	})
	channel, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Publish(budget(t), channel, []byte(savedPayload))
	if err != nil || got != (Publication{Status: 200, MessageID: messageID, MayHaveSent: true}) || publications.Load() != 1 {
		t.Fatalf("publication result=%+v error=%v", got, err)
	}
}
