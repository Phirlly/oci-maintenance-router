package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRootInputsDoNotDefaultTestAuthority(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("variables_operation.tf")
	if err != nil {
		t.Fatal(err)
	}
	for _, explicitNull := range []bool{false, true} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), source, 0600); err != nil {
			t.Fatal(err)
		}
		if explicitNull {
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"mode": nil, "test_run": nil, "test_secret_ocid": nil, "test_nonproduction": nil})
		}
		output, err := terraform(t, dir, "jsonencode({mode=var.mode,run=var.test_run,secret=var.test_secret_ocid,attestation=var.test_nonproduction})\n", "console", "-no-color")
		var encoded string
		var values map[string]any
		if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &values) != nil {
			t.Fatalf("root defaults: %s (%v)", output, err)
		}
		if values["mode"] != "PREVIEW" || values["run"] != nil || values["secret"] != "" || values["attestation"] != false {
			t.Fatalf("missing root input granted test authority: %#v", values)
		}
	}
}
