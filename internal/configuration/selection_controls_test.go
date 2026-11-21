package configuration

import "testing"

func TestSelectedConfigurationRetainsSetupAndRecoveryRules(t *testing.T) {
	for _, tc := range []struct{ mode, action string }{{"PREVIEW", "NONE"}, {"PAUSED", "NONE"}, {"LIVE", "NONE"}, {"TEST", "SEND_TEST"}, {"TEST", "CONFIRM"}} {
		v := setupSettings(t, tc.mode, tc.action)
		v["schemaVersion"], v["resourceTypes"] = 9, []string{"instance"}
		delete(v, "mysqlServices")
		c := selectedConfig(t, v)
		if c.Mode != tc.mode || c.Setup.Action != tc.action || c.OperatorChannel == nil {
			t.Fatal("setup controls changed")
		}
	}
	for _, edit := range []func(map[string]any){
		func(v map[string]any) { delete(v, "setup") },
		func(v map[string]any) { v["setup"] = nil },
		func(v map[string]any) { v["mode"] = "TEST" },
		func(v map[string]any) { v["mode"] = "LIVE" },
		func(v map[string]any) { v["diagnostic"] = map[string]any{} },
		func(v map[string]any) { v["operatorTest"] = map[string]any{} },
		func(v map[string]any) { v["preparation"] = map[string]any{"initialize": "true"} },
		func(v map[string]any) { v["recovery"] = map[string]any{} },
	} {
		v := selectionSettings(t)
		edit(v)
		if _, err := Decode(encoded(t, v), selectionCatalog(t)); err == nil {
			t.Fatal("invalid controls accepted")
		}
	}
}

func TestSelectedConfigurationPreservesExplicitRecoveryIntent(t *testing.T) {
	for _, tc := range []struct{ mode, kind string }{{"PAUSED", "CLOSE_BATCH"}, {"LIVE", "RETRY_BATCH"}} {
		v := setupSettings(t, tc.mode, "NONE")
		v["schemaVersion"], v["resourceTypes"] = 9, []string{"instance"}
		delete(v, "mysqlServices")
		v["recovery"] = recoverySettings(t, tc.kind)["recovery"]
		c := selectedConfig(t, v)
		if c.Recovery == nil || c.Recovery.Kind != tc.kind {
			t.Fatal("recovery intent lost")
		}
	}
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		v := setupSettings(t, "TEST", action)
		v["schemaVersion"], v["resourceTypes"] = 9, []string{"instance"}
		delete(v, "mysqlServices")
		v["recovery"] = recoverySettings(t, "RETRY_BATCH")["recovery"]
		if _, err := Decode(encoded(t, v), selectionCatalog(t)); err == nil {
			t.Fatal("test plus recovery accepted")
		}
	}
}
