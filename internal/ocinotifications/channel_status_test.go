package ocinotifications

import (
	"net/http"
	"sync/atomic"
	"testing"
)

func TestChannelReadsReject202EvenWithValidBody(t *testing.T) {
	for _, path := range []string{"/20181201/topics/" + topicID, "/20181201/subscriptions/" + subscriptionID} {
		t.Run(path, func(t *testing.T) {
			var calls atomic.Int32
			c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == path {
					w.WriteHeader(http.StatusAccepted)
				}
				serveChannel(w, r)
			})
			got, err := c.GetChannel(budget(t), topicID, subscriptionID)
			wantEmpty(t, got)
			wantError(t, err, Service, 202, false)
			wantCalls := int32(1)
			if path == "/20181201/subscriptions/"+subscriptionID {
				wantCalls = 2
			}
			if calls.Load() != wantCalls {
				t.Fatal("failed read retried or continued")
			}
		})
	}
}
