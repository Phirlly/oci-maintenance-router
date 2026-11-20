package configuration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSetupAttemptRequiresExactBoundedPositiveInteger(t *testing.T) {
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		for _, value := range []any{json.Number("0"), json.Number("-1"), json.Number("1.0"), json.Number("1e0"), json.Number("2147483648"), json.Number("999999999999999999999999")} {
			v := setupSettings(t, "TEST", action)
			v["setup"].(map[string]any)["attempt"] = value
			wantError(t, encoded(t, v), "setup.attempt", "invalid")
		}
		for _, value := range []any{nil, "1", true, []any{}, map[string]any{}} {
			v := setupSettings(t, "TEST", action)
			v["setup"].(map[string]any)["attempt"] = value
			wantError(t, encoded(t, v), "setup.attempt", "type")
		}
		for _, n := range []int64{1, 2147483647} {
			v := setupSettings(t, "TEST", action)
			v["setup"].(map[string]any)["attempt"] = n
			if decoded(t, v).Setup.Attempt != n {
				t.Fatal("attempt coerced")
			}
		}
	}
}

func TestSetupVariantsRejectCrossActionInputsAndPartialConfirmation(t *testing.T) {
	for _, tc := range []struct{ action, field string }{
		{"NONE", "attempt"}, {"NONE", "secretId"}, {"NONE", "pagerDutyCode"},
		{"SEND_TEST", "pagerDutyCode"}, {"SEND_TEST", "operatorCode"},
		{"CONFIRM", "secretId"}, {"CONFIRM", "nonProduction"},
	} {
		mode := "TEST"
		if tc.action == "NONE" {
			mode = "PAUSED"
		}
		v := setupSettings(t, mode, tc.action)
		v["setup"].(map[string]any)[tc.field] = "private-value"
		wantError(t, encoded(t, v), "setup", "unknown_field")
	}
	for _, field := range []string{"pagerDutyCode", "operatorCode"} {
		v := setupSettings(t, "TEST", "CONFIRM")
		delete(v["setup"].(map[string]any), field)
		wantError(t, encoded(t, v), "setup."+field, "required")
		for _, code := range []string{"", "private", strings.Repeat("x", 100)} {
			v = setupSettings(t, "TEST", "CONFIRM")
			v["setup"].(map[string]any)[field] = code
			wantError(t, encoded(t, v), "setup."+field, "invalid")
		}
	}
}

func TestSetupRetainsConsentRegionalSecretAndChannelValidation(t *testing.T) {
	for _, field := range []string{"installationId", "mode"} {
		v := setupSettings(t, "LIVE", "NONE")
		v[field] = "invalid input"
		wantError(t, encoded(t, v), field, "invalid")
	}
	for _, tc := range []struct {
		field string
		value any
		code  string
	}{
		{"nonProduction", false, "invalid"}, {"nonProduction", "true", "type"},
		{"secretId", "ocid1.vaultsecret.oc1.phx.wrong", "invalid"},
		{"secretId", strings.Repeat("x", 4097), "invalid"}, {"secretId", nil, "type"},
	} {
		v := setupSettings(t, "TEST", "SEND_TEST")
		v["setup"].(map[string]any)[tc.field] = tc.value
		wantError(t, encoded(t, v), "setup."+tc.field, tc.code)
	}
	for _, tc := range []struct{ field, value string }{{"subscriptionId", "wrong"}, {"endpoint", ""}, {"endpoint", "with space"}} {
		v := setupSettings(t, "LIVE", "NONE")
		v["operatorChannel"].(map[string]any)[tc.field] = tc.value
		wantError(t, encoded(t, v), "operatorChannel."+tc.field, "invalid")
	}
}
