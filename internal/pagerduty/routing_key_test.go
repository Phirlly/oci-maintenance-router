package pagerduty

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidRoutingKeyMatchesExistingWireRule(t *testing.T) {
	for _, text := range []string{syntheticKey, strings.Repeat("界", 32), strings.Repeat("😀", 32), strings.Repeat("\"", 32), strings.Repeat("\n", 32), strings.Repeat(" ", 32)} {
		key := []byte(text)
		before := bytes.Clone(key)
		if !ValidRoutingKey(key) {
			t.Fatal("valid key rejected")
		}
		if _, err := envelope([]byte(savedEvent), key); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(key, before) {
			t.Fatal("key modified")
		}
	}
	for _, key := range [][]byte{nil, {}, []byte(strings.Repeat("a", 31)), []byte(strings.Repeat("a", 33)), []byte(strings.Repeat("😀", 33)), bytes.Repeat([]byte{255}, 32)} {
		if ValidRoutingKey(key) {
			t.Fatal("invalid key admitted")
		}
		if _, err := envelope([]byte(savedEvent), key); err == nil {
			t.Fatal("wire validator disagrees")
		}
	}
}
