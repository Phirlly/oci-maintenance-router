package ocinotifications

import (
	"encoding/json"
	"strings"
	"testing"
)

func changeField(body, name string, value any, remove bool) []byte {
	var object map[string]any
	json.Unmarshal([]byte(body), &object)
	if remove {
		delete(object, name)
	} else {
		object[name] = value
	}
	result, _ := json.Marshal(object)
	return result
}
func TestChannelIdentityFieldsRejectMalformedOrForeignData(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	topic, _ := c.decodeTopic([]byte(topicJSON), topicID)
	for _, source := range []struct {
		body   string
		fields []string
		decode func([]byte) (Channel, error)
	}{
		{topicJSON, []string{"topicId", "compartmentId", "lifecycleState", "apiEndpoint"}, func(b []byte) (Channel, error) { return c.decodeTopic(b, topicID) }},
		{subscriptionJSON, []string{"id", "topicId", "compartmentId", "lifecycleState", "protocol", "endpoint"}, func(b []byte) (Channel, error) { return c.decodeSubscription(b, topic, subscriptionID) }},
	} {
		for _, name := range source.fields {
			for _, bad := range []any{nil, false, 42, []any{}, map[string]any{}, ""} {
				ch, err := source.decode(changeField(source.body, name, bad, false))
				wantEmpty(t, ch)
				if err == nil {
					t.Fatal("malformed field accepted")
				}
			}
			ch, err := source.decode(changeField(source.body, name, nil, true))
			wantEmpty(t, ch)
			if err == nil {
				t.Fatal("missing field accepted")
			}
			ch, err = source.decode([]byte(strings.TrimSuffix(source.body, "}") + `,"` + strings.ToUpper(name) + `":"alias"}`))
			wantEmpty(t, ch)
			if err == nil {
				t.Fatal("alias accepted")
			}
		}
		for _, bad := range []string{"null", "[]", "{}", `{"data":` + source.body + `}`} {
			ch, err := source.decode([]byte(bad))
			wantEmpty(t, ch)
			if err == nil {
				t.Fatal("wrapper accepted")
			}
		}
		for _, bad := range []string{"ocid1.tenancy.oc1..foreign", "ocid1.compartment.oc2..x", "ocid1.compartment.oc1.iad.x", strings.Repeat("x", 4097)} {
			ch, err := source.decode(changeField(source.body, "compartmentId", bad, false))
			wantEmpty(t, ch)
			if err == nil {
				t.Fatal("invalid compartment accepted")
			}
		}
		for _, state := range []string{"PENDING", "DELETING", "DELETED", "FUTURE"} {
			ch, err := source.decode(changeField(source.body, "lifecycleState", state, false))
			wantEmpty(t, ch)
			wantError(t, err, InactiveChannel, 0, false)
		}
	}
	for _, name := range []string{"id", "topicId", "protocol"} {
		ch, err := c.decodeSubscription(changeField(subscriptionJSON, name, "foreign", false), topic, subscriptionID)
		wantEmpty(t, ch)
		if err == nil {
			t.Fatal("association changed")
		}
	}
	ch, err := c.decodeTopic(changeField(topicJSON, "topicId", topicID+"other", false), topicID)
	wantEmpty(t, ch)
	if err == nil {
		t.Fatal("wrong topic accepted")
	}
}

func TestEndpointBoundsAndExactTargetBytes(t *testing.T) {
	c, _ := New("us-ashburn-1", "iad", tenancy, &unusedProvider{})
	topic, _ := c.decodeTopic([]byte(topicJSON), topicID)
	for _, target := range []string{"Team@example.invalid", strings.Repeat("😀", 512), strings.Repeat("a", 512)} {
		ch, err := c.decodeSubscription(changeField(subscriptionJSON, "endpoint", target, false), topic, subscriptionID)
		if err != nil || ch.Snapshot().Endpoint != target {
			t.Fatal("target bytes changed/rejected")
		}
	}
	for _, target := range []string{" ", "a b", "a\nb", "a\u0000b", "a\u0085b", "a\u00a0b", strings.Repeat("a", 513), strings.Repeat("😀", 513)} {
		ch, err := c.decodeSubscription(changeField(subscriptionJSON, "endpoint", target, false), topic, subscriptionID)
		wantEmpty(t, ch)
		if err == nil {
			t.Fatal("invalid target accepted")
		}
	}
	if opaqueText(string([]byte{255}), 2048) {
		t.Fatal("invalid UTF8 admitted")
	}
	root, err := c.decodeTopic(changeField(topicJSON, "compartmentId", tenancy, false), topicID)
	if err != nil || root.Snapshot().TopicCompartmentID != tenancy {
		t.Fatal("root scope rejected")
	}
}
