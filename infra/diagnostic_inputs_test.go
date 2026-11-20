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
	for _, tc := range []struct {
		name        string
		inputs      map[string]any
		compartment string
	}{
		{name: "omitted", inputs: map[string]any{}},
		{name: "null", inputs: map[string]any{"routing_state": nil, "setup_action": nil, "setup_attempt": nil, "test_secret_ocid": nil, "test_nonproduction": nil, "test_secret_compartment_ocid": nil}},
		{name: "explicit-compartment", inputs: map[string]any{"test_secret_compartment_ocid": "ocid1.compartment.oc1..test"}, compartment: "ocid1.compartment.oc1..test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "variables.tf"), source, 0600); err != nil {
				t.Fatal(err)
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), tc.inputs)
			output, err := terraform(t, dir, "jsonencode({state=var.routing_state,action=var.setup_action,run=var.setup_attempt,secret=var.test_secret_ocid,attestation=var.test_nonproduction,compartment=var.test_secret_compartment_ocid})\n", "console", "-no-color")
			var encoded string
			var values map[string]any
			if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &values) != nil {
				t.Fatalf("root defaults: %s (%v)", output, err)
			}
			if values["state"] != "Paused" || values["action"] != "None" || values["run"] != nil || values["secret"] != "" || values["attestation"] != false {
				t.Fatalf("missing root input granted test authority: %#v", values)
			}
			if values["compartment"] != tc.compartment {
				t.Fatalf("test compartment default or explicit selection changed: %#v", values)
			}
		})
	}
}
