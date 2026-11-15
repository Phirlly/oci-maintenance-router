package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootOperatorInputsCannotCreateMalformedSubscription(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("variables_operator.tf")
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []any{nil, "", "Ops+Router@Example.invalid", "ops\u00a0@example.invalid", "missing-at", "ops@@example.invalid", strings.Repeat("a\u0338", 251) + "@example.io"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), source, 0600); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"operator_email": email})
		writeJSON(t, filepath.Join(dir, "outputs.tf.json"), map[string]any{"output": map[string]any{"operator": map[string]any{"value": map[string]string{"email": "${var.operator_email}", "run": "${var.operator_test_run}"}}}})
		if output, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
			t.Fatalf("init: %s (%v)", output, err)
		}
		output, err := terraform(t, dir, "", "plan", "-refresh=false", "-input=false", "-no-color", "-out=result.tfplan")
		valid := email == nil || email == "" || email == "Ops+Router@Example.invalid"
		if !valid {
			if err == nil || !strings.Contains(string(output), "Operator email") {
				t.Fatalf("root failed to reject malformed address: %s (%v)", output, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("valid root input failed: %s (%v)", output, err)
		}
		output, err = terraform(t, dir, "", "show", "-json", "result.tfplan")
		var plan struct {
			Planned struct {
				Outputs map[string]struct{ Value map[string]any }
			} `json:"planned_values"`
		}
		if err != nil || json.Unmarshal(output, &plan) != nil {
			t.Fatalf("root defaults: %s (%v)", output, err)
		}
		values := plan.Planned.Outputs["operator"].Value
		want := email
		if want == nil {
			want = ""
		}
		if values["email"] != want || values["run"] != nil {
			t.Fatalf("root changed intent: %#v", values)
		}
	}
}
