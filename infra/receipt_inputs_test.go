package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReceiptRootDefaultsAndWiringPreserveIntent(t *testing.T) {
	requireTerraform(t)
	wiring := readChecksumSource(t, "configuration.tf")
	if !regexp.MustCompile(`receipt_confirmation\s*=\s*module.configuration.receipt_confirmation`).MatchString(wiring) {
		t.Fatal("permissions and results must use the module's raw-input confirmation intent")
	}
	for _, tc := range []struct{ file, name string }{
		{"variables_operation.tf", "test_receipt_code"},
		{"variables_operator.tf", "operator_receipt_code"},
	} {
		if !regexp.MustCompile(tc.name + `\s*=\s*var\.` + tc.name).MatchString(wiring) {
			t.Fatal("receipt input is not wired to renderer", tc.name)
		}
		for _, kind := range []string{"omitted", "null", "exact", "inactive-malformed"} {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(readChecksumSource(t, tc.file)), 0600); err != nil {
					t.Fatal(err)
				}
				want := ""
				inputs := map[string]any{}
				switch kind {
				case "null":
					inputs[tc.name] = nil
				case "exact":
					want = receiptSettings()[tc.name].(string)
					inputs[tc.name] = want
				case "inactive-malformed":
					want = "inactive receipt"
					inputs[tc.name] = want
				}
				writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), inputs)
				writeJSON(t, filepath.Join(dir, "output.tf.json"), map[string]any{"output": map[string]any{"code": map[string]string{"value": "${var." + tc.name + "}"}}})
				if out, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
					t.Fatalf("init: %s (%v)", out, err)
				}
				if out, err := terraform(t, dir, "", "plan", "-refresh=false", "-input=false", "-no-color", "-out=result.tfplan"); err != nil {
					t.Fatalf("inactive input must not block recovery: %s (%v)", out, err)
				}
				out, err := terraform(t, dir, "", "show", "-json", "result.tfplan")
				var plan struct {
					Planned struct {
						Outputs map[string]struct{ Value string }
					} `json:"planned_values"`
				}
				if err != nil || json.Unmarshal(out, &plan) != nil || plan.Planned.Outputs["code"].Value != want {
					t.Fatalf("receipt input changed: %s (%v)", out, err)
				}
			})
		}
	}
}
