package infra_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// These are retained provider-free Terraform outputs, not managed delivery proof.
func TestSetupRetainedStateDoesNotRestoreHiddenIntent(t *testing.T) {
	dir := t.TempDir()
	source, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{
		"variable": map[string]any{"settings": map[string]string{"type": "any"}},
		"module": map[string]any{"configuration": map[string]any{
			"source": source, "settings": "${var.settings}", "catalog": catalogFixture(), "installation_id": "b1-" + strings.Repeat("a", 64),
			"operator_subscription_id": "ocid1.onssubscription.oc1.iad.operator",
			"topics":                   map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"},
		}},
		"output": map[string]any{"configuration": map[string]string{"value": "${module.configuration.configuration_json}"}},
	})
	if out, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		t.Fatalf("init: %s (%v)", out, err)
	}
	var previous string
	catalog, err := json.Marshal(catalogFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"legacy", "upgrade-default", "send", "repeat", "confirm-missing", "confirm", "none", "enabled", "return-to-confirm", "new-attempt", "rollback", "reupgrade"} {
		t.Run(step, func(t *testing.T) {
			s := setupSettings("TEST", "SEND_TEST", true)
			switch step {
			case "legacy", "rollback":
				s = operatorSettings()
			case "upgrade-default", "none", "reupgrade":
				s = setupSettings("PREVIEW", "NONE", true)
				s["test_receipt_code"], s["operator_receipt_code"] = "stale", "stale"
				s["test_run"], s["operator_test_run"] = 7, 9
			case "enabled":
				s = setupSettings("LIVE", "NONE", true)
			case "confirm", "confirm-missing", "return-to-confirm":
				s = setupSettings("TEST", "CONFIRM", true)
				if step != "confirm" {
					delete(s, "operator_receipt_code")
				}
			case "new-attempt":
				s["setup_attempt"] = 2
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": s})
			out, err := terraform(t, dir, "", "apply", "-auto-approve", "-input=false", "-no-color")
			if step == "confirm-missing" || step == "return-to-confirm" {
				if err == nil || !strings.Contains(string(out), "Receipt confirmation") {
					t.Fatalf("omitted confirmation restored or fell back to send: %s (%v)", out, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply: %s (%v)", out, err)
			}
			out, err = terraform(t, dir, "", "output", "-json", "configuration")
			var raw string
			if err != nil || json.Unmarshal(out, &raw) != nil {
				t.Fatalf("output: %s (%v)", out, err)
			}
			c := renderedConfig(t, map[string]string{"configuration": raw, "catalog": string(catalog)})
			if step == "repeat" && raw != previous {
				t.Fatal("repeat changed the explicit selected attempt")
			}
			if s["setup_action"] == "NONE" && (c.Setup == nil || c.Setup.Action != "NONE" || c.Setup.Attempt != 0 || c.Diagnostic != nil || c.OperatorTest != nil) {
				t.Fatal("inactive/migrated setup retained transient authority")
			}
			if step == "new-attempt" && (c.Setup == nil || c.Setup.Attempt != 2 || c.Setup.Action != "SEND_TEST") {
				t.Fatal("explicit new attempt was not preserved")
			}
			previous = raw
		})
	}
}
