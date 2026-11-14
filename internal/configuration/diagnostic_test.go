package configuration

import (
	"strings"
	"testing"
)

func diagnosticSettings(t *testing.T) map[string]any {
	t.Helper()
	v := preparationSettings(t)
	v["schemaVersion"] = 3
	v["mode"] = "TEST"
	v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true}
	return v
}
func TestDiagnosticControlsRequireExplicitCompleteIntent(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		v := diagnosticSettings(t)
		v["mode"] = mode
		c := decoded(t, v)
		if c.SchemaVersion != 3 || c.Mode != mode || c.InstallationID != "installation-1" || !c.Preparation.Initialize || c.Diagnostic == nil || *c.Diagnostic != (DiagnosticRequest{"test-1", "ocid1.vaultsecret.oc1.iad.test", true}) || c.ActiveBaselineID != "" || len(c.Replay) != 0 {
			t.Fatal("incorrect TEST controls")
		}
		second := decoded(t, v)
		c.Diagnostic.RequestID = "changed"
		v["diagnostic"].(map[string]any)["secretId"] = "changed"
		if second.Diagnostic.RequestID != "test-1" || second.Diagnostic.SecretID != "ocid1.vaultsecret.oc1.iad.test" {
			t.Fatal("decoded intent aliases another caller")
		}
		delete(v, "diagnostic")
		if mode == "TEST" {
			wantError(t, encoded(t, v), "diagnostic", "required_for_test")
		} else if decoded(t, v).Diagnostic != nil {
			t.Fatal("absent intent became requested")
		}
	}
	for _, v := range []map[string]any{settings(t), preparationSettings(t)} {
		if decoded(t, v).Diagnostic != nil {
			t.Fatal("legacy configuration gained diagnostic intent")
		}
	}
}

func TestDiagnosticControlsRejectMissingNullCoercedAndUnknownFields(t *testing.T) {
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := diagnosticSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
	for _, field := range []string{"requestId", "secretId", "nonProduction"} {
		v := diagnosticSettings(t)
		delete(v["diagnostic"].(map[string]any), field)
		wantError(t, encoded(t, v), "diagnostic."+field, "required")
		v = diagnosticSettings(t)
		v["diagnostic"].(map[string]any)[field] = nil
		wantError(t, encoded(t, v), "diagnostic."+field, "type")
	}
	for _, value := range []any{nil, true, "private", []any{}} {
		v := diagnosticSettings(t)
		v["diagnostic"] = value
		wantError(t, encoded(t, v), "diagnostic", "type")
	}
	for _, value := range []any{"true", 1, map[string]any{}} {
		v := diagnosticSettings(t)
		v["diagnostic"].(map[string]any)["nonProduction"] = value
		wantError(t, encoded(t, v), "diagnostic.nonProduction", "type")
	}
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		v := diagnosticSettings(t)
		v["mode"] = mode
		v["diagnostic"].(map[string]any)["nonProduction"] = false
		wantError(t, encoded(t, v), "diagnostic.nonProduction", "invalid")
	}
	for _, name := range []string{"payload", "secretVersion", "receiptConfirmed", "RequestId", "private-name"} {
		v := diagnosticSettings(t)
		v["diagnostic"].(map[string]any)[name] = "private-value"
		wantError(t, encoded(t, v), "diagnostic", "unknown_field")
	}
	for _, mode := range []string{"LIVE", "test", ""} {
		v := diagnosticSettings(t)
		v["mode"] = mode
		wantError(t, encoded(t, v), "mode", "invalid")
	}
	for _, field := range []string{"activeBaselineId", "replay"} {
		for _, value := range []any{nil, "", []any{}} {
			v := diagnosticSettings(t)
			v[field] = value
			wantError(t, encoded(t, v), "configuration", "unknown_field")
		}
	}
	for _, v := range []map[string]any{settings(t), preparationSettings(t)} {
		v["diagnostic"] = diagnosticSettings(t)["diagnostic"]
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
}

func TestDiagnosticRequestAndSecretBoundaries(t *testing.T) {
	for _, id := range []string{"", "../id", "with space", "é", strings.Repeat("a", 129)} {
		v := diagnosticSettings(t)
		v["diagnostic"].(map[string]any)["requestId"] = id
		wantError(t, encoded(t, v), "diagnostic.requestId", "invalid")
	}
	for _, id := range []string{"wrong", "ocid1.vaultsecret.oc2.iad.test", "ocid1.vaultsecret.oc1.phx.test", "ocid1.vaultsecret.oc1..test", "ocid1.vaultsecret.oc1.iad.", "ocid1.db-system.oc1.iad.test"} {
		v := diagnosticSettings(t)
		v["diagnostic"].(map[string]any)["secretId"] = id
		wantError(t, encoded(t, v), "diagnostic.secretId", "invalid")
	}
	for _, id := range []string{"ocid1.vaultsecret.oc1.iad.test", "ocid1.vaultsecret.oc1.us-ashburn-1.test", "ocid1.vaultsecret.oc1.iad..test", "ocid1.vaultsecret.oc1.iad.future.test"} {
		v := diagnosticSettings(t)
		v["diagnostic"].(map[string]any)["secretId"] = id
		v["diagnostic"].(map[string]any)["requestId"] = strings.Repeat("A", 128)
		if decoded(t, v).Diagnostic.SecretID != id {
			t.Fatal("secret spelling changed")
		}
	}
	prefix := "ocid1.vaultsecret.oc1.iad."
	id := prefix + strings.Repeat("a", 4096-len(prefix))
	v := diagnosticSettings(t)
	v["diagnostic"].(map[string]any)["secretId"] = id
	if decoded(t, v).Diagnostic.SecretID != id {
		t.Fatal("maximum reference rejected")
	}
	v["diagnostic"].(map[string]any)["secretId"] = id + "a"
	wantError(t, encoded(t, v), "diagnostic.secretId", "invalid")
}
