package ocistorage

import (
	"strings"
	"testing"
)

func TestHistoryWireJSONNeverHidesAmbiguousOrMalformedFields(t *testing.T) {
	for _, body := range [][]byte{
		nil, []byte("{"), []byte(`{"items":[]} {}`), []byte("{\"items\":[],\"future\":\"\xff\"}"),
		[]byte(`{"items":[],"items":[]}`), []byte(`{"items":[],"\u0069tems":[]}`),
		[]byte(`{"items":[],"future":{"x":0,"x":1}}`),
		[]byte(`{"items":[],"future":"\ud800"}`), []byte(`{"items":[],"future":"\udfff"}`),
	} {
		empty, err := decodeHistoryEmpty(body, "control/head.json")
		wantError(t, err, InvalidResponse, 0, false)
		if empty {
			t.Fatal("ambiguous wire data established absence")
		}
	}
}

func TestHistoryWireLimitsRemainBoundedIncludingUnknownMetadata(t *testing.T) {
	for _, value := range []string{
		`"` + strings.Repeat("x", objectLimit) + `"`,
		strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32),
		"[" + strings.Repeat("0,", listingValueLimit) + "0]",
	} {
		empty, err := decodeHistoryEmpty([]byte(`{"items":[],"future":`+value+`}`), "control/head.json")
		wantError(t, err, TooLarge, 0, false)
		if empty {
			t.Fatal("over-limit response established absence")
		}
	}
}
