package configuration

import "testing"

func liveSettings(t *testing.T) map[string]any {
	t.Helper()
	v := operatorSettings(t)
	v["schemaVersion"] = 6
	v["mode"] = "LIVE"
	return v
}
func TestLiveControlsRequireExplicitModeAndPreserveOwnedIntents(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		v := liveSettings(t)
		v["mode"] = mode
		c := decoded(t, v)
		if c.SchemaVersion != 6 || c.Mode != mode || c.Diagnostic.ReceiptCode != receiptCode || c.OperatorTest.ReceiptCode != operatorCode || c.ActiveBaselineID != "" || len(c.Replay) != 0 {
			t.Fatal("schema6 lost explicit controls")
		}
		v["diagnostic"].(map[string]any)["receiptCode"] = "changed"
		v["operatorTest"].(map[string]any)["endpoint"] = "changed"
		if c.Diagnostic.ReceiptCode != receiptCode || c.OperatorTest.Endpoint != "Team@example.invalid" {
			t.Fatal("decoded controls alias input")
		}
	}
	for _, field := range []string{"mode", "installationId", "preparation"} {
		v := liveSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
}
func TestLiveControlsRequireBothReceiptInputs(t *testing.T) {
	for _, field := range []string{"diagnostic", "operatorTest"} {
		v := liveSettings(t)
		delete(v, field)
		wantError(t, encoded(t, v), field, "required_for_live")
		for _, omit := range []bool{false, true} {
			v := liveSettings(t)
			o := v[field].(map[string]any)
			o["receiptCode"] = ""
			if omit {
				delete(o, "receiptCode")
			}
			wantError(t, encoded(t, v), field+".receiptCode", "required_for_live")
		}
		for _, bad := range []any{nil, true, 1, []any{}} {
			v := liveSettings(t)
			v[field].(map[string]any)["receiptCode"] = bad
			wantError(t, encoded(t, v), field+".receiptCode", "type")
		}
		v = liveSettings(t)
		v[field].(map[string]any)["receiptCode"] = "private-invalid"
		wantError(t, encoded(t, v), field+".receiptCode", "invalid")
		v = liveSettings(t)
		v[field] = nil
		wantError(t, encoded(t, v), field, "type")
	}
}
func TestLiveSchemaPreservesNonsendingAndTestInputRules(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED"} {
		v := liveSettings(t)
		v["mode"] = mode
		delete(v, "diagnostic")
		delete(v, "operatorTest")
		c := decoded(t, v)
		if c.Diagnostic != nil || c.OperatorTest != nil {
			t.Fatal("absent controls invented")
		}
	}
	v := liveSettings(t)
	v["mode"] = "TEST"
	delete(v, "operatorTest")
	delete(v["diagnostic"].(map[string]any), "receiptCode")
	if decoded(t, v).Diagnostic.ReceiptCode != "" {
		t.Fatal("TEST requires prior receipt")
	}
	delete(v, "diagnostic")
	wantError(t, encoded(t, v), "diagnostic", "required_for_test")
	v = liveSettings(t)
	v["diagnostic"].(map[string]any)["nonProduction"] = false
	wantError(t, encoded(t, v), "diagnostic.nonProduction", "invalid")
	for _, mode := range []string{"live", "", "ACTIVE"} {
		v := liveSettings(t)
		v["mode"] = mode
		wantError(t, encoded(t, v), "mode", "invalid")
	}
}
