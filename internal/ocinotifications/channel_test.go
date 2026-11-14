package ocinotifications

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConstructorUsesFixedOriginWithoutReadingCredentials(t *testing.T) {
	c, err := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	if err != nil || c == nil || c.endpoint != controlOrigin || c.regionKey != "iad" || c.tenancy != tenancy {
		t.Fatalf("regional construction failed: %v", err)
	}
}

func TestChannelUsesReturnedOriginAndExactAssociation(t *testing.T) {
	key := generatedKey(t)
	var calls atomic.Int32
	c, _ := tlsClient(t, syntheticKey{key: key}, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		verifySignature(t, r, key, nil)
		if r.Method != "GET" || r.URL.RawQuery != "" || !r.Close || r.ProtoMajor != 1 || r.Header.Get("Accept") != "application/json" {
			t.Error("wrong read contract")
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Error("cached metadata request")
		}
		if n%2 == 1 {
			if r.Host != strings.TrimPrefix(controlOrigin, "https://") || r.URL.Path != "/20181201/topics/"+topicID {
				t.Error("wrong topic read")
			}
			io.WriteString(w, topicJSON)
		} else {
			if r.Host != strings.TrimPrefix(dataOrigin, "https://") || r.URL.Path != "/20181201/subscriptions/"+subscriptionID {
				t.Error("guessed or wrong data origin")
			}
			io.WriteString(w, subscriptionJSON)
		}
	})
	for range 2 {
		channel, err := c.GetChannel(budget(t), topicID, subscriptionID)
		want := ChannelSnapshot{TopicID: topicID, SubscriptionID: subscriptionID, TopicCompartmentID: compartment, SubscriptionCompartmentID: tenancy, Protocol: "EMAIL", Endpoint: "Team@example.invalid"}
		if err != nil || channel.Snapshot() != want {
			t.Fatalf("channel not bound: %v", err)
		}
		snapshot := channel.Snapshot()
		snapshot.Endpoint = "caller change"
		if channel.Snapshot() != want {
			t.Fatal("shared snapshot")
		}
		for _, text := range []string{fmt.Sprint(channel), fmt.Sprintf("%+v", channel), fmt.Sprintf("%#v", channel)} {
			if strings.Contains(text, "example") || strings.Contains(text, "https") {
				t.Fatal("channel display leaked data")
			}
		}
	}
	if calls.Load() != 4 {
		t.Fatal("metadata was cached or retried")
	}
}
