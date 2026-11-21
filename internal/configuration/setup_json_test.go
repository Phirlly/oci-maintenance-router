package configuration

import (
	"strings"
	"testing"
)

func TestSetupStrictJSONAndIndependentOwnedControls(t *testing.T) {
	v := setupSettings(t, "TEST", "SEND_TEST")
	wire := string(encoded(t, v))
	for _, key := range []string{"action", `act\u0069on`} {
		body := strings.Replace(wire, `"action":"SEND_TEST"`, `"action":"SEND_TEST","`+key+`":"NONE"`, 1)
		wantError(t, []byte(body), "configuration", "duplicate_key")
	}
	for _, version := range []string{"8.0", "8e0", `"8"`, "10"} {
		wantError(t, []byte(strings.Replace(wire, `"schemaVersion":8`, `"schemaVersion":`+version, 1)), "schemaVersion", "unsupported")
	}
	for _, value := range []any{nil, true, "private", []any{}} {
		v = setupSettings(t, "TEST", "SEND_TEST")
		v["setup"] = value
		wantError(t, encoded(t, v), "setup", "type")
	}
	v = setupSettings(t, "TEST", "SEND_TEST")
	a, b := decoded(t, v), decoded(t, v)
	a.Setup.SecretID, a.OperatorChannel.Endpoint = "changed", "changed"
	v["setup"].(map[string]any)["attempt"] = 3
	if b.Setup.Attempt != 1 || b.Setup.SecretID != "ocid1.vaultsecret.oc1.iad.test" || b.OperatorChannel.Endpoint != "Team@example.invalid" {
		t.Fatal("configuration aliases input or another decode")
	}
}

func TestSetupRecoveryIsSeparateFromTestsAndIdentity(t *testing.T) {
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		v := setupSettings(t, "TEST", action)
		v["recovery"] = recoverySettings(t, "RETRY_BATCH")["recovery"]
		wantError(t, encoded(t, v), "recovery", "forbidden_during_test")
	}
	for _, tc := range []struct{ mode, kind string }{{"PAUSED", "CLOSE_BATCH"}, {"LIVE", "RETRY_BATCH"}} {
		v := setupSettings(t, tc.mode, "NONE")
		v["recovery"] = recoverySettings(t, tc.kind)["recovery"]
		if decoded(t, v).Recovery.Kind != tc.kind {
			t.Fatal("existing explicit recovery lost")
		}
	}
}
