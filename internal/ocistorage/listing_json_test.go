package ocistorage

import (
	"strings"
	"testing"
)

func TestListingInvalidWireJSONIsRejected(t *testing.T) {
	for _, extra := range []string{
		`,"objects":[]`, `,"\u006fbjects":[]`, `,"future":{"x":0,"x":1}`,
		`,"future":{"nested":{"name":"one","name":"two"}}`,
		`,"future":{"name":"one","\u006eame":"two"}`,
		`,"future":"\ud800"`, `,"future":"\udfff"`, `,"future":"\ud800\u0041"`, `,"future":"\ud800x"`,
	} {
		m, err := decodeNoticePage([]byte(strings.TrimSuffix(`{"objects":[]}`, "}") + extra + "}"))
		if err == nil || m.Names != nil {
			t.Errorf("invalid JSON accepted: %q", extra)
		}
	}
	for _, body := range [][]byte{nil, []byte("{"), []byte(`{"objects":[]}` + "{}"), []byte("{\"objects\":[],\"future\":\"\xff\"}")} {
		if _, err := decodeNoticePage(body); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}

func TestListingAdditiveUnicodeRemainsCompatible(t *testing.T) {
	for _, value := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"\u0041\n\t\""`, `1e999`, strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31)} {
		body := strings.TrimSuffix(`{"objects":[]}`, "}") + `,"future":` + value + "}"
		if _, err := decodeNoticePage([]byte(body)); err != nil {
			t.Fatalf("valid additive metadata rejected: %v", err)
		}
	}
}

func TestListingWireAndTreeLimits(t *testing.T) {
	for _, suffix := range []string{
		`,"future":"` + strings.Repeat("x", 1<<20) + `"`,
		`,"future":` + strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32),
		`,"future":[` + strings.Repeat("0,", 65536) + "0]",
	} {
		m, err := decodeNoticePage([]byte(strings.TrimSuffix(`{"objects":[]}`, "}") + suffix + "}"))
		if err == nil || m.Names != nil {
			t.Fatal("unbounded metadata accepted")
		}
	}
}
