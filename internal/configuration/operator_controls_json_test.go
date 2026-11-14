package configuration

import (
	"strings"
	"testing"
)

func TestOperatorControlsRetainStrictJSONAndPrivateErrors(t *testing.T) {
	wire := string(encoded(t, operatorSettings(t)))
	for _, tc := range []struct{ old, replacement, field, code string }{
		{`"requestId":"operator-1"`, `"requestId":"operator-1","requestId":"private"`, "configuration", "duplicate_key"},
		{`"endpoint":"Team@example.invalid"`, `"endpoint":"a","\u0065ndpoint":"private"`, "configuration", "duplicate_key"},
		{"Team@example.invalid", `\ud800`, "configuration", "invalid_json"},
		{"Team@example.invalid", "\xff", "configuration", "invalid_json"},
		{`"operatorTest"`, `"OperatorTest"`, "configuration", "unknown_field"},
		{`"subscriptionId"`, `"SubscriptionId"`, "operatorTest.subscriptionId", "required"},
		{`"receiptCode":"` + operatorCode + `"`, `"ReceiptCode":"` + operatorCode + `"`, "operatorTest", "unknown_field"},
	} {
		body := []byte(strings.Replace(wire, tc.old, tc.replacement, 1))
		wantError(t, body, tc.field, tc.code)
		_, err := Decode(body, catalog())
		if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "example.invalid") {
			t.Fatal("input exposed in parse error")
		}
	}
	wantError(t, append([]byte(wire), []byte("{}")...), "configuration", "invalid_json")
	wantError(t, append([]byte(wire), []byte(strings.Repeat(" ", maxConfigBytes))...), "configuration", "too_large")
}
