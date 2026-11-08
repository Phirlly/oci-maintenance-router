package infra_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func terraform(t *testing.T, dir, input string, args ...string) ([]byte, error) {
	t.Helper()
	bin := os.Getenv("TERRAFORM_BIN")
	if bin == "" {
		bin = "terraform"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// Keep caller credentials, CLI injection and backend/plugin configuration out.
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(key, "TF_") || strings.HasPrefix(key, "OCI_") {
			continue
		}
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=1", "TF_INPUT=0", "CHECKPOINT_DISABLE=1")
	cmd.Stdin = strings.NewReader(input)
	return cmd.CombinedOutput()
}

func requireTerraform(t *testing.T) {
	t.Helper()
	output, err := terraform(t, t.TempDir(), "", "version", "-json")
	var version struct {
		Version string `json:"terraform_version"`
	}
	if err != nil || json.Unmarshal(output, &version) != nil || version.Version != "1.5.7" {
		t.Fatalf("tests require Terraform1.5.7 via TERRAFORM_BIN: %s (%v)", output, err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func render(t *testing.T, settings map[string]any) (map[string]string, string, error) {
	t.Helper()
	requireTerraform(t)
	dir := t.TempDir()
	source, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{
		"module": map[string]any{"configuration": map[string]any{
			"source": source, "settings": "${var.settings}", "catalog": catalogFixture(),
			"topics": map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"},
		}},
		"variable": map[string]any{"settings": map[string]string{"type": "any"}},
		"output": map[string]any{
			"configuration": map[string]string{"value": "${module.configuration.configuration_json}"},
			"catalog":       map[string]string{"value": "${module.configuration.catalog_json}"},
		},
	})
	writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": settings})
	if output, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
		t.Fatalf("init: %s %v", output, err)
	}
	output, err := terraform(t, dir, "", "plan", "-refresh=false", "-input=false", "-no-color", "-out=result.tfplan")
	if err != nil {
		return nil, string(output), err
	}
	output, err = terraform(t, dir, "", "show", "-json", "result.tfplan")
	if err != nil {
		t.Fatalf("show: %s %v", output, err)
	}
	var plan struct {
		Planned struct {
			Outputs map[string]struct{ Value string }
		} `json:"planned_values"`
	}
	if err := json.Unmarshal(output, &plan); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for key, value := range plan.Planned.Outputs {
		values[key] = value.Value
	}
	return values, "", nil
}

func catalogFixture() map[string]any {
	return map[string]any{"regions": map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"},
		"mysqlServices": []string{"Oracle Cloud Infrastructure HeatWave MySQL", "Oracle HeatWave MySQL"}}
}

func settingsFixture() map[string]any {
	return map[string]any{
		"environment_name": "example", "tenancy_ocid": "ocid1.tenancy.oc1..example", "region": "us-ashburn-1",
		"mysql_regions": []string{"us-phoenix-1"}, "mysql_compartments": []string{"ocid1.compartment.oc1..example"},
		"ownership_kind": "defined", "ownership_namespace": "Operations", "ownership_key": "Owner",
		"destinations": map[string]string{"payments": "ocid1.vaultsecret.oc1.iad.example"},
		"routes":       map[string]string{"payments": "payments", "billing": "payments"}, "pagerduty_region": "US",
	}
}
