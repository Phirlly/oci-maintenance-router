package configuration

import (
	"strings"
	"testing"
)

func TestReceiptJSONRetainsStrictParsingAndControlRules(t *testing.T) {
	wire := string(encoded(t, receiptSettings(t)))
	for _, tc := range []struct{ old, value, field, code string }{
		{`"receiptCode":"` + receiptCode + `"`, `"receiptCode":"","receiptCode":"private"`, "configuration", "duplicate_key"},
		{receiptCode, `\ud800`, "configuration", "invalid_json"},
		{receiptCode, "\xff", "configuration", "invalid_json"},
		{`"receiptCode"`, `"ReceiptCode"`, "diagnostic", "unknown_field"},
		{`"TEST"`, `"LIVE"`, "mode", "invalid"},
	} {
		body := []byte(strings.Replace(wire, tc.old, tc.value, 1))
		wantError(t, body, tc.field, tc.code)
		_, err := Decode(body, catalog())
		if strings.Contains(err.Error(), "private") {
			t.Fatal("input leaked in error")
		}
	}
	for _, version := range []string{"4.0", "4e0", `"4"`, "6"} {
		wantError(t, []byte(strings.Replace(wire, `"schemaVersion":4`, `"schemaVersion":`+version, 1)), "schemaVersion", "unsupported")
	}
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := receiptSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
	for _, field := range []string{"activeBaselineId", "replay"} {
		v := receiptSettings(t)
		v[field] = nil
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
	for _, field := range []string{"requestId", "secretId", "nonProduction"} {
		v := receiptSettings(t)
		delete(v["diagnostic"].(map[string]any), field)
		wantError(t, encoded(t, v), "diagnostic."+field, "required")
	}
}
