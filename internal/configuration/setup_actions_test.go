package configuration

import "testing"

func TestSetupConfigurationAdmitsOnlyExplicitModeActionPairs(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		for _, action := range []string{"NONE", "SEND_TEST", "CONFIRM"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				v := setupSettings(t, mode, action)
				valid := mode == "TEST" && action != "NONE" || mode != "TEST" && action == "NONE"
				c, err := Decode(encoded(t, v), catalog())
				if valid {
					if err != nil || c.SchemaVersion != 8 || c.Mode != mode || c.Diagnostic != nil || c.OperatorTest != nil {
						t.Fatal("explicit setup lost or legacy intent synthesized", err)
					}
				} else if err == nil {
					t.Fatal("incompatible action admitted")
				}
			})
		}
	}
}

func TestSetupNoneOmitsTransientIntentWithoutActivatingLegacyProof(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "LIVE"} {
		v := setupSettings(t, mode, "NONE")
		c := decoded(t, v)
		if c.Diagnostic != nil || c.OperatorTest != nil || c.ActiveBaselineID != "" || len(c.Replay) != 0 {
			t.Fatal("NONE fabricated legacy authority")
		}
		delete(v, "operatorChannel")
		_, err := Decode(encoded(t, v), catalog())
		if (err == nil) != (mode != "LIVE") {
			t.Fatal("wrong channel requirement", err)
		}
	}
}

func TestSetupRejectsImplicitOrLegacyTestIntent(t *testing.T) {
	for _, field := range []string{"setup", "mode", "installationId", "preparation"} {
		v := setupSettings(t, "TEST", "SEND_TEST")
		delete(v, field)
		wantError(t, encoded(t, v), field, "required")
	}
	for _, field := range []string{"diagnostic", "operatorTest", "activeBaselineId", "replay"} {
		v := setupSettings(t, "TEST", "SEND_TEST")
		v[field] = nil
		wantError(t, encoded(t, v), "configuration", "unknown_field")
	}
	for _, action := range []string{"", "send_test", "LIVE", "private"} {
		v := setupSettings(t, "TEST", "SEND_TEST")
		v["setup"].(map[string]any)["action"] = action
		wantError(t, encoded(t, v), "setup.action", "invalid")
	}
}
