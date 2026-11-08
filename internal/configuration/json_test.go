package configuration

import (
	"bytes"
	"strings"
	"testing"
)

func TestRejectMalformedJSON(t *testing.T) {
	cases := map[string][]byte{
		"empty": nil, "unfinished": []byte("{"), "trailing": []byte("{} {}"),
		"comma": []byte("{\"a\":1,}"), "invalid utf8": []byte{'{', '"', 0xff, '"', ':', '1', '}'},
		"high surrogate":   []byte(`{"a":"\ud800"}`),
		"low surrogate":    []byte(`{"a":"\udc00"}`),
		"wrong pair":       []byte(`{"a":"\ud800\u0041"}`),
		"interrupted pair": []byte(`{"a":"\ud800x\udc00"}`),
		"too deep":         []byte(strings.Repeat("[", 33) + strings.Repeat("]", 33)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) { wantError(t, data, "configuration", "invalid_json") })
	}
}

func TestRejectDuplicateDecodedKeys(t *testing.T) {
	for _, data := range []string{
		`{"mode":"PREVIEW","mode":"LIVE"}`,
		`{"mode":"PREVIEW","\u006dode":"LIVE"}`,
		`{"ownership":{"kind":"freeform","key":"A","key":"B"}}`,
		`{"routes":[{"ownerValue":"A","ownerValue":"B"}]}`,
	} {
		wantError(t, []byte(data), "configuration", "duplicate_key")
	}
}

func TestJSONSizeBoundary(t *testing.T) {
	base := fixture(t)
	exact := append(append([]byte{}, base...), bytes.Repeat([]byte(" "), (1<<20)-len(base))...)
	if _, err := Decode(exact, catalog()); err != nil {
		t.Fatal(err)
	}
	wantError(t, append(exact, ' '), "configuration", "too_large")
}

func TestNonObjectRoot(t *testing.T) {
	for _, data := range []string{"[]", "null", "true", "1", "\"text\""} {
		wantError(t, []byte(data), "configuration", "type")
	}
}

func TestValidEscapedUnicodeAndBackslashes(t *testing.T) {
	value := settings(t)
	value["routes"] = []any{map[string]any{"ownerValue": "marker", "destinationId": "payments"}}
	for input, want := range map[string]string{
		`\ud83d\udc2c`:         "🐬",
		`\\ud800`:              `\ud800`,
		`\"quoted\"\n\t\u0041`: "\"quoted\"\n\tA",
	} {
		data := bytes.Replace(encoded(t, value), []byte("marker"), []byte(input), 1)
		got, err := Decode(data, catalog())
		if err != nil {
			t.Fatal(err)
		}
		if got.Routes[0].OwnerValue != want {
			t.Fatalf("owner = %q, want %q", got.Routes[0].OwnerValue, want)
		}
	}
}

func TestErrorsDoNotEchoInput(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`{"sensitive-property":"sensitive-value"}`),
		[]byte(`{"sensitive-property":"sensitive-value","sensitive-property":1}`),
		bytes.Replace(fixture(t), []byte(`"US"`), []byte(`"sensitive-value"`), 1),
	} {
		_, err := Decode(data, catalog())
		if err == nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestJSONContainerDepthBoundary(t *testing.T) {
	data := []byte(strings.Repeat("[", 32) + "null" + strings.Repeat("]", 32))
	if _, err := strictJSON(data); err != nil {
		t.Fatal(err)
	}
}
