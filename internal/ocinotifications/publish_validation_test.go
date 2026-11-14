package ocinotifications

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

func TestPublishRejectsMissingForeignChannelAndInvalidPayloadBeforeSigning(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, serveChannel)
	channel, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	c.signer = common.DefaultRequestSigner(&unusedProvider{})
	foreign, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	for _, test := range []struct {
		client  *Client
		channel Channel
	}{{nil, channel}, {c, Channel{}}, {foreign, channel}, {&Client{}, channel}} {
		r, err := test.client.Publish(budget(t), test.channel, []byte(savedPayload))
		wantError(t, err, InvalidRequest, 0, false)
		if r != (Publication{}) {
			t.Fatal("invalid request reported a dispatch")
		}
	}
	for _, body := range []string{
		"", "{", "null", "[]", `{}`, `{"body":"text"}`, `{"body":"","title":"x"}`,
		`{"body":null,"title":"x"}`, `{"body":"x","title":null}`,
		`{"body":"x","title":"x","future":true}`, `{"body":"x","title":"x","body":"other"}`,
		`{"body":"x","Title":"x"}`, savedPayload + "{}", `{"body":"\ud800","title":"x"}`,
	} {
		r, err := c.Publish(budget(t), channel, []byte(body))
		wantError(t, err, InvalidRequest, 0, false)
		if r != (Publication{}) {
			t.Fatal("invalid body dispatched")
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		code := DeadlineRequired
		if ctx == nil {
			code = InvalidRequest
		}
		r, err := c.Publish(ctx, channel, []byte(savedPayload))
		wantError(t, err, code, 0, false)
		if r != (Publication{}) {
			t.Fatal("invalid lifetime dispatched")
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	r, err := c.Publish(ctx, channel, []byte(savedPayload))
	wantError(t, err, DeadlineExceeded, 0, false)
	if r != (Publication{}) {
		t.Fatal("late request dispatched")
	}
	r, err = c.Publish(budget(t), channel, make([]byte, messageLimit+1))
	wantError(t, err, TooLarge, 0, false)
	if r != (Publication{}) {
		t.Fatal("oversized request dispatched")
	}
}

func TestMessageEncodedSizeAndTitleCharacterBoundaries(t *testing.T) {
	for _, n := range []int{255, 256} {
		body, _ := json.Marshal(map[string]string{"body": "test", "title": strings.Repeat("😀", n)})
		if validMessage(body) != (n == 255) {
			t.Fatal("incorrect title character limit")
		}
	}
	prefix := `{"title":"","body":"`
	suffix := `"}`
	payload := []byte(prefix + strings.Repeat("a", messageLimit-len(prefix)-len(suffix)) + suffix)
	var publications atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			serveChannel(w, r)
			return
		}
		publications.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Method != http.MethodPost || r.ContentLength != int64(messageLimit) || !bytes.Equal(body, payload) {
			t.Errorf("boundary request: method=%s length=%d received=%d error=%v", r.Method, r.ContentLength, len(body), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"messageId":"` + messageID + `"}`))
	})
	ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := c.Publish(budget(t), ch, payload); err != nil || r != (Publication{Status: http.StatusOK, MessageID: messageID, MayHaveSent: true}) {
		t.Fatalf("exact encoded bound: result=%+v error=%v", r, err)
	}
	if publications.Load() != 1 {
		t.Fatalf("publication count=%d, want one", publications.Load())
	}
}
