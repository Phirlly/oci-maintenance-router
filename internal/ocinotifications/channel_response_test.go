package ocinotifications

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSubscriptionFailureDiscardsValidTopic(t *testing.T) {
	key := generatedKey(t)
	for _, kind := range []string{"service", "malformed", "wrong association", "inactive"} {
		var calls atomic.Int32
		c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path == "/20181201/topics/"+topicID {
				io.WriteString(w, topicJSON)
				return
			}
			switch kind {
			case "service":
				w.WriteHeader(404)
			case "malformed":
				io.WriteString(w, "{")
			case "wrong association":
				io.WriteString(w, strings.Replace(subscriptionJSON, topicID, topicID+"other", 1))
			case "inactive":
				io.WriteString(w, strings.Replace(subscriptionJSON, "ACTIVE", "PENDING", 1))
			}
		})
		ch, err := c.GetChannel(budget(t), topicID, subscriptionID)
		code, status := InvalidResponse, 200
		if kind == "service" {
			code, status = Service, 404
		}
		if kind == "inactive" {
			code = InactiveChannel
		}
		wantEmpty(t, ch)
		wantError(t, err, code, status, false)
		if calls.Load() != 2 {
			t.Fatal("failed association retried")
		}
	}
}

func TestSafeRegionalIdentifierVariantsRetainExactRequests(t *testing.T) {
	c, _ := tlsClient(t, syntheticKey{key: generatedKey(t)}, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
		if strings.HasPrefix(r.URL.Path, "/20181201/topics/") {
			io.WriteString(w, strings.ReplaceAll(topicJSON, topicID, id))
			return
		}
		variant := strings.TrimPrefix(id, "ocid1.onssubscription.oc1.")
		body := strings.ReplaceAll(subscriptionJSON, subscriptionID, id)
		io.WriteString(w, strings.ReplaceAll(body, topicID, "ocid1.onstopic.oc1."+variant))
	})
	for _, variant := range []string{"iad..example", "us-ashburn-1.example", "iad.future.example-A_1"} {
		topic, subscription := "ocid1.onstopic.oc1."+variant, "ocid1.onssubscription.oc1."+variant
		ch, err := c.GetChannel(budget(t), topic, subscription)
		if err != nil || ch.Snapshot().TopicID != topic || ch.Snapshot().SubscriptionID != subscription {
			t.Fatal("safe exact ID rejected/changed")
		}
	}
}
