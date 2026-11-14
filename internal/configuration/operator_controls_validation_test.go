package configuration

import (
	"strings"
	"testing"
)

func TestOperatorControlsRejectIncompleteAndCoercedInput(t *testing.T) {
	for _, field := range []string{"requestId", "subscriptionId", "endpoint"} {
		v := operatorSettings(t)
		delete(v["operatorTest"].(map[string]any), field)
		wantError(t, encoded(t, v), "operatorTest."+field, "required")
	}
	for _, field := range []string{"requestId", "subscriptionId", "endpoint", "receiptCode"} {
		for _, value := range []any{nil, true, 1, []any{}, map[string]any{}} {
			v := operatorSettings(t)
			v["operatorTest"].(map[string]any)[field] = value
			wantError(t, encoded(t, v), "operatorTest."+field, "type")
		}
	}
	for _, value := range []any{nil, true, 1, "private", []any{}} {
		v := operatorSettings(t)
		v["operatorTest"] = value
		wantError(t, encoded(t, v), "operatorTest", "type")
	}
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := operatorSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
}

func TestOperatorControlsRejectExtraFieldsWithoutLeakingThem(t *testing.T) {
	for _, field := range []string{"payload", "protocol", "topicId", "receiptConfirmed", "RequestId", "private-name"} {
		v := operatorSettings(t)
		v["operatorTest"].(map[string]any)[field] = "private-value"
		body := encoded(t, v)
		wantError(t, body, "operatorTest", "unknown_field")
		_, err := Decode(body, catalog())
		if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "example.invalid") {
			t.Fatal("input leaked into error")
		}
	}
	for _, mode := range []string{"LIVE", "test", ""} {
		v := operatorSettings(t)
		v["mode"] = mode
		wantError(t, encoded(t, v), "mode", "invalid")
	}
	for _, field := range []string{"activeBaselineId", "replay"} {
		v := operatorSettings(t)
		v[field] = nil
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
}

func TestEarlierSchemasRejectOperatorTestAndNeverInventIntent(t *testing.T) {
	for _, v := range []map[string]any{settings(t), preparationSettings(t), diagnosticSettings(t), receiptSettings(t)} {
		if decoded(t, v).OperatorTest != nil {
			t.Fatal("legacy configuration gained operator intent")
		}
		v["operatorTest"] = operatorSettings(t)["operatorTest"]
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
}
