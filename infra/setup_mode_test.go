package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCustomerControlsMapToExistingRuntimeOperations(t *testing.T) {
	data, err := os.ReadFile("setup.tf")
	if err != nil {
		t.Fatal("missing customer control mapping", err)
	}
	for _, tc := range []struct {
		state, action, want string
		closure             bool
	}{
		{"Paused", "None", "PREVIEW", false},
		{"Paused", "None", "PAUSED", true},
		{"Paused", "Send test", "TEST", false},
		{"Paused", "Confirm received tests", "TEST", false},
		{"Enabled", "None", "LIVE", false},
	} {
		t.Run(tc.state+"/"+tc.action+"/"+tc.want, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "setup.tf"), data, 0600); err != nil {
				t.Fatal(err)
			}
			vars := map[string]any{}
			for _, name := range []string{"routing_state", "setup_action", "closure_confirm", "operator_email"} {
				vars[name] = map[string]string{"type": "any"}
			}
			writeJSON(t, filepath.Join(dir, "variables.tf.json"), map[string]any{"variable": vars})
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"routing_state": tc.state, "setup_action": tc.action, "closure_confirm": tc.closure, "operator_email": ""})
			output, err := terraform(t, dir, "local.mode\n", "console", "-no-color")
			var mode string
			if err != nil || json.Unmarshal(output, &mode) != nil || mode != tc.want {
				t.Fatalf("mode: %s (%v), want %s", output, err, tc.want)
			}
		})
	}
}
