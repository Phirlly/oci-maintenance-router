package infra_test

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Retained provider-free state proves input/identity behavior, not cloud delivery.
func TestReceiptLifecyclePreservesRunsAndNeverRestoresHiddenAuthority(t *testing.T) {
	requireTerraform(t)
	dir := t.TempDir()
	source, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^\s*installation_id\s*=\s*(.*bucket_id.*)$`).FindStringSubmatch(readChecksumSource(t, "configuration.tf"))
	if len(match) != 2 {
		t.Fatal("installation expression missing")
	}
	identity := strings.ReplaceAll(match[1], "oci_objectstorage_bucket.state.bucket_id", "terraform_data.bucket.output")
	writeBundle := func(legacy bool) {
		t.Helper()
		bundle := map[string]any{
			"variable": map[string]any{"settings": map[string]string{"type": "any"}},
			"resource": map[string]any{"terraform_data": map[string]any{"bucket": map[string]string{"input": "ocid1.bucket.oc1.iad.retained"}}},
			"output":   map[string]any{"configuration": map[string]string{"value": "{}"}, "confirmation": map[string]bool{"value": false}},
		}
		if !legacy {
			bundle["module"] = map[string]any{"configuration": map[string]any{
				"source": source, "settings": "${var.settings}", "catalog": catalogFixture(), "installation_id": "${" + identity + "}",
				"operator_subscription_id": "ocid1.onssubscription.oc1.iad.operator",
				"topics":                   map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"},
			}}
			bundle["output"] = map[string]any{
				"configuration": map[string]string{"value": "${module.configuration.configuration_json}"},
				"confirmation":  map[string]string{"value": "${module.configuration.receipt_confirmation}"},
			}
		}
		writeJSON(t, filepath.Join(dir, "main.tf.json"), bundle)
		if out, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
			t.Fatalf("init: %s (%v)", out, err)
		}
	}
	writeBundle(false)
	s := operatorSettings()
	first := map[string]string{}
	previous := map[string]string{}
	for _, step := range []string{"send", "confirm", "repeat", "routes", "partial", "retry", "orphan", "restore-email", "paused-malformed", "omitted", "reenter", "confirm-again", "changed-run-stale-code", "clear-codes-new-run", "legacy", "reupgrade"} {
		t.Run(step, func(t *testing.T) {
			switch step {
			case "confirm", "retry", "confirm-again":
				s["test_receipt_code"], s["operator_receipt_code"] = receiptSettings()["test_receipt_code"], receiptSettings()["operator_receipt_code"]
			case "routes":
				s["routes"] = map[string]string{"new-owner": "payments"}
			case "partial":
				delete(s, "operator_receipt_code")
			case "orphan":
				delete(s, "operator_email")
			case "restore-email":
				s["operator_email"] = operatorSettings()["operator_email"]
			case "paused-malformed":
				s["mode"], s["test_receipt_code"], s["operator_receipt_code"] = "PAUSED", "stale", "stale"
			case "omitted":
				delete(s, "test_receipt_code")
				delete(s, "operator_receipt_code")
			case "reenter":
				s["mode"] = "TEST"
			case "changed-run-stale-code":
				s["test_run"], s["operator_test_run"] = 20, 20
			case "clear-codes-new-run":
				s["test_receipt_code"], s["operator_receipt_code"] = "", ""
				s["test_run"], s["operator_test_run"] = 21, 21
			case "legacy":
				writeBundle(true)
			case "reupgrade":
				writeBundle(false)
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": s})
			out, err := terraform(t, dir, "", "apply", "-auto-approve", "-input=false", "-no-color")
			if step == "partial" || step == "orphan" {
				if err == nil || !strings.Contains(string(out), "Receipt confirmation") {
					t.Fatalf("incomplete confirmation allowed: %s (%v)", out, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply: %s (%v)", out, err)
			}
			out, err = terraform(t, dir, "", "output", "-json")
			var outputs map[string]struct{ Value any }
			if err != nil || json.Unmarshal(out, &outputs) != nil {
				t.Fatalf("outputs: %s (%v)", out, err)
			}
			var config map[string]any
			if err := json.Unmarshal([]byte(outputs["configuration"].Value.(string)), &config); err != nil {
				t.Fatal(err)
			}
			inactive := step == "paused-malformed" || step == "omitted" || step == "legacy"
			wantConfirmation := !inactive && s["test_receipt_code"] != nil && s["test_receipt_code"] != ""
			if outputs["confirmation"].Value != wantConfirmation {
				t.Fatal("receipt intent was restored or discarded")
			}
			for _, channel := range []string{"diagnostic", "operatorTest"} {
				if inactive {
					if config[channel] != nil {
						t.Fatal("inactive/legacy bundle retained test authority")
					}
					continue
				}
				proof := config[channel].(map[string]any)
				id := proof["requestId"].(string)
				if step == "send" {
					first[channel] = id
				}
				newRun := step == "changed-run-stale-code" || step == "clear-codes-new-run" || step == "reupgrade"
				if (id != first[channel]) != newRun || (proof["receiptCode"] != nil) != wantConfirmation {
					t.Fatal("receipt edits changed run identity or restored an omitted code")
				}
				if step == "changed-run-stale-code" || step == "clear-codes-new-run" {
					if id == previous[channel] {
						t.Fatal("explicit new run reused the previous request identity")
					}
				} else if step != "send" && id != previous[channel] {
					t.Fatal("mode, receipt or package edit changed the retained request")
				}
				previous[channel] = id
			}
		})
	}
}
