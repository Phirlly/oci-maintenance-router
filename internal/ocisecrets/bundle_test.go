package ocisecrets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBundleOwnsBytesAndRedactsOrdinaryFormatting(t *testing.T) {
	want := []byte("synthetic-private-key-material")
	b, err := decodeBundle(bundleBody(t, testSecretID, 7, want), testSecretID, 7)
	if err != nil || b.Version() != 7 || !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("complete bundle rejected: %v", err)
	}
	first := b.Bytes()
	first[0] = 'X'
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatal("caller mutated retained secret bytes")
	}
	for _, text := range []string{fmt.Sprint(b), fmt.Sprintf("%+v", b), fmt.Sprintf("%#v", b), fmt.Sprint(&b), fmt.Sprintf("%#v", &b)} {
		if !strings.Contains(text, "redacted") || strings.Contains(text, string(want)) {
			t.Fatal("ordinary formatting exposes secret material")
		}
	}
	serialized, err := json.Marshal(b)
	if err != nil || string(serialized) != "{}" {
		t.Fatal("default JSON serialized bundle contents")
	}
	wantEmpty(t, Bundle{})
}

func TestPrivateContainersCannotExposeBundleContentsThroughFormatting(t *testing.T) {
	secret := []byte("synthetic-private-key-material")
	b, err := decodeBundle(bundleBody(t, testSecretID, 7, secret), testSecretID, 7)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := struct{ bundle Bundle }{b}
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		text := fmt.Sprintf(format, wrapped)
		// Private fields bypass formatting methods; reject both plaintext and
		// recoverable element encodings instead of relying on a redaction label.
		for _, encoded := range []string{string(secret), fmt.Sprint(secret), fmt.Sprintf("%#v", secret), fmt.Sprintf("%x", secret)} {
			if strings.Contains(text, encoded) {
				t.Fatalf("%s exposes secret contents", format)
			}
		}
	}
}
