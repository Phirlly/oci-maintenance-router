package ociownership

import (
	"strings"
	"testing"
)

func TestInvalidWireJSONIsRejected(t *testing.T) {
	for _, extra := range []string{
		`,"id":"duplicate"`, `,"\u0069d":"escaped alias"`, `,"future":{"x":0,"x":1}`,
		`,"definedTags":{"Operations":{"Owner":"payments","Owner":"video"}}`,
		`,"freeformTags":{"Owner":"payments","\u004fwner":"video"}`,
		`,"future":"\ud800"`, `,"future":"\udfff"`, `,"future":"\ud800\u0041"`, `,"future":"\ud800x"`,
	} {
		m, err := decodeMetadata([]byte(strings.TrimSuffix(minimal, "}")+extra+"}"), dbID, tenancy)
		if err == nil || m.ID != nil {
			t.Errorf("invalid JSON accepted: %q", extra)
		}
	}
	for _, body := range [][]byte{nil, []byte("{"), []byte(minimal + "{}"), []byte(strings.Replace(minimal, "example", string([]byte{0xff}), 1))} {
		if _, err := decodeMetadata(body, dbID, tenancy); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}

func TestValidUnicodeAndUnknownMetadataRemainCompatible(t *testing.T) {
	for _, value := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"\u0041\n\t\""`, `1e999`, strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31)} {
		body := strings.TrimSuffix(minimal, "}") + `,"future":` + value + "}"
		if _, err := decodeMetadata([]byte(body), dbID, tenancy); err != nil {
			t.Fatalf("valid additive metadata rejected: %v", err)
		}
	}
}

func TestWireAndTreeLimits(t *testing.T) {
	for _, suffix := range []string{
		`,"future":"` + strings.Repeat("x", 2<<20) + `"`,
		`,"future":` + strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32),
		`,"future":[` + strings.Repeat("0,", 65536) + "0]",
	} {
		m, err := decodeMetadata([]byte(strings.TrimSuffix(minimal, "}")+suffix+"}"), dbID, tenancy)
		if err == nil || m.ID != nil {
			t.Fatal("unbounded metadata accepted")
		}
	}
}
