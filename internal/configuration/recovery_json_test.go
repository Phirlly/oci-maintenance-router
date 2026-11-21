package configuration

import (
	"strings"
	"testing"
)

func TestRecoveryControlsRejectExtraFieldsAndEarlierSchemas(t *testing.T) {
	for _, field := range []string{"payload", "secretId", "secretVersion", "force", "approved", "RequestID", "private-name"} {
		v := recoverySettings(t, "RETRY_BATCH")
		v["recovery"].(map[string]any)[field] = "private-value"
		body := encoded(t, v)
		wantError(t, body, "recovery", "unknown_field")
		_, err := Decode(body, catalog())
		if strings.Contains(err.Error(), "private") {
			t.Fatal("input leaked")
		}
	}
	for _, v := range []map[string]any{settings(t), preparationSettings(t), diagnosticSettings(t), receiptSettings(t), operatorSettings(t), liveSettings(t)} {
		if decoded(t, v).Recovery != nil {
			t.Fatal("legacy config invented request")
		}
		v["recovery"] = recoverySettings(t, "RETRY_BATCH")["recovery"]
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
}

func TestRecoveryControlsRetainStrictJSON(t *testing.T) {
	wire := string(encoded(t, recoverySettings(t, "RETRY_BATCH")))
	for _, version := range []string{"10", "7.0", "7e0", `"7"`} {
		wantError(t, []byte(strings.Replace(wire, `"schemaVersion":7`, `"schemaVersion":`+version, 1)), "schemaVersion", "unsupported")
	}
	for _, key := range []string{`requestId`, `request\u0049d`} {
		body := strings.Replace(wire, `"requestId":"recovery-1"`, `"requestId":"recovery-1","`+key+`":"recovery-2"`, 1)
		wantError(t, []byte(body), "configuration", "duplicate_key")
	}
	wantError(t, []byte(strings.Replace(wire, `"requestId":"recovery-1"`, `"RequestId":"recovery-1"`, 1)), "recovery.requestId", "required")
	wantError(t, []byte(strings.Replace(wire, `"requestId":"recovery-1"`, `"requestId":"\ud800"`, 1)), "configuration", "invalid_json")
	wantError(t, []byte(wire+" {}"), "configuration", "invalid_json")
	for _, field := range []string{"activeBaselineId", "replay", "ready", "recoveryActions"} {
		v := recoverySettings(t, "RETRY_BATCH")
		v[field] = nil
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
}
