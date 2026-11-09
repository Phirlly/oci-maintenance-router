package pagerduty

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const savedEvent = "{\"dedup_key\":\"synthetic-batch\",\"event_action\":\"trigger\",\"payload\":{\"summary\":\"Maintenance\",\"source\":\"synthetic\",\"severity\":\"info\"}}"
const syntheticKey = "synthetic-integration-key-000001"

func TestEnvelopePreservesSavedBytesAndEscapesKey(t *testing.T) {
	for _, key := range []string{syntheticKey, strings.Repeat("界", 32), strings.Repeat("\"", 32), strings.Repeat("\n", 32), strings.Repeat(" ", 32)} {
		saved := []byte(savedEvent)
		keyBytes := []byte(key)
		body, err := envelope(saved, keyBytes)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(key)
		prefix := append([]byte("{\"routing_key\":"), encoded...)
		prefix = append(prefix, ',')
		if !bytes.Equal(body, append(prefix, saved[1:]...)) {
			t.Fatal("saved bytes rebuilt or key escaped incorrectly")
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(body, &got); err != nil || len(got) != 4 {
			t.Fatal("invalid outgoing JSON")
		}
		var gotKey string
		if json.Unmarshal(got["routing_key"], &gotKey) != nil || gotKey != key {
			t.Fatal("key altered")
		}
		saved[1] = 'x'
		keyBytes[0] = 'x'
		if !bytes.Contains(body, []byte("\"dedup_key\"")) || bytes.Equal(body, keyBytes) {
			t.Fatal("output aliases inputs")
		}
	}
}

func TestEnvelopeRejectsInvalidInputs(t *testing.T) {
	for _, input := range []string{"", "{}", "null", "[]", savedEvent + " ", " " + savedEvent, savedEvent + "{}", strings.Replace(savedEvent, "trigger", "resolve", 1), strings.Replace(savedEvent, "synthetic-batch", "", 1), strings.Replace(savedEvent, "synthetic-batch", strings.Repeat("x", 256), 1), strings.Replace(savedEvent, "\"trigger\"", "null", 1), strings.Replace(savedEvent, "\"synthetic-batch\"", "7", 1), "{\"dedup_key\":\"x\",\"event_action\":\"trigger\",\"payload\":null}", "{\"dedup_key\":\"x\",\"event_action\":\"trigger\",\"payload\":[]}", strings.Replace(savedEvent, "{", "{\"routing_key\":\"private\",", 1), strings.Replace(savedEvent, "{", "{\"dedup_key\":\"other\",", 1), strings.Replace(savedEvent, "{", "{\"unknown\":0,", 1), "{\"dedup_key\":\"\xff\"}", "{broken}"} {
		body, err := envelope([]byte(input), []byte(syntheticKey))
		var e *Error
		if body != nil || !errors.As(err, &e) || e.Code != InvalidRequest || e.MayHaveSent {
			t.Fatalf("invalid envelope accepted: %v", err)
		}
	}
	for _, key := range [][]byte{nil, []byte(strings.Repeat("x", 31)), []byte(strings.Repeat("x", 33)), []byte(strings.Repeat("界", 33)), bytes.Repeat([]byte{255}, 32)} {
		body, err := envelope([]byte(savedEvent), key)
		if body != nil || err == nil {
			t.Fatal("invalid key accepted")
		}
	}
}

func TestEnvelopeEnforcesFinalWireByteLimit(t *testing.T) {
	key := []byte(syntheticKey)
	base, err := envelope([]byte(savedEvent), key)
	if err != nil {
		t.Fatal(err)
	}
	fitting := strings.Replace(savedEvent, "Maintenance", "Maintenance"+strings.Repeat("x", requestLimit-len(base)), 1)
	got, err := envelope([]byte(fitting), key)
	if err != nil || len(got) != requestLimit {
		t.Fatal("exact wire boundary rejected")
	}
	for _, value := range []string{strings.Replace(fitting, "Maintenance", "Maintenancex", 1), strings.Repeat("x", requestLimit+1)} {
		got, err := envelope([]byte(value), key)
		var e *Error
		if got != nil || !errors.As(err, &e) || e.Code != TooLarge {
			t.Fatal("oversize request not rejected")
		}
	}
}
