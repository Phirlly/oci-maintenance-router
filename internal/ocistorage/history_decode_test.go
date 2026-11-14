package ocistorage

import (
	"strings"
	"testing"
)

func TestHistoryEmptyAcceptsCompleteOptionalPrefixShapes(t *testing.T) {
	for _, body := range []string{
		`{"items":[]}`, `{"items":[],"prefixes":null}`, `{"items":[],"prefixes":[]}`,
		`{"items":[],"future":{"text":"\ud83d\ude00","number":1e999}}`,
	} {
		empty, err := decodeHistoryEmpty([]byte(body), "control/head.json")
		if err != nil || !empty {
			t.Fatalf("complete response rejected: %v, %v", empty, err)
		}
	}
}

func TestHistoryRetainedVersionsAndPrefixCollisionsAreNotEmpty(t *testing.T) {
	for _, item := range []string{
		`{"name":"control/head.json","versionId":"v1","isDeleteMarker":false}`,
		`{"name":"control/head.json","versionId":"v2","isDeleteMarker":true}`,
		`{"name":"control/head.json.backup"}`, // Conservative refusal, not exact-key absence.
	} {
		empty, err := decodeHistoryEmpty([]byte(`{"items":[`+item+`]}`), "control/head.json")
		if err != nil || empty {
			t.Fatalf("retained prefix result = %v, %v", empty, err)
		}
	}
}

func TestHistoryMalformedShapesNeverEstablishAbsence(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"items":null}`, `{"items":{}}`,
		`{"Items":[]}`, `{"items":[],"Items":[]}`, `{"items":[],"Prefixes":[]}`,
		`{"items":[],"prefixes":["control/"]}`, `{"items":[],"prefixes":""}`,
		`{"items":[null]}`, `{"items":[{}]}`, `{"items":[{"name":null}]}`,
		`{"items":[{"name":""}]}`, `{"items":[{"name":"other/head.json"}]}`,
		`{"items":[{"name":"control/head.json","Name":"control/head.json"}]}`,
		`{"items":[{"name":"control/head.json\n"}]}`,
		`{"items":[{"name":"control/head.json` + strings.Repeat("x", 1025) + `"}]}`,
	} {
		empty, err := decodeHistoryEmpty([]byte(body), "control/head.json")
		wantError(t, err, InvalidResponse, 0, false)
		if empty {
			t.Fatal("invalid response established absence")
		}
	}

	empty, err := decodeHistoryEmpty([]byte(`{"items":[{"name":"control/head.json"},{"name":"control/head.json"}]}`), "control/head.json")
	wantError(t, err, TooLarge, 0, false)
	if empty {
		t.Fatal("over-limit response established absence")
	}
}
