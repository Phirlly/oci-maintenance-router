package deployment

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

func closureTerraform(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	bin := os.Getenv("TERRAFORM_BIN")
	if bin == "" {
		t.Fatal("TERRAFORM_BIN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if !strings.HasPrefix(key, "TF_") && !strings.HasPrefix(key, "OCI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=1", "TF_INPUT=0", "CHECKPOINT_DISABLE=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Terraform %v: %s (%v)", args, b, err)
	}
	return b
}

func renderProducedClosure(t *testing.T, f savedReviewFixture, code string) []byte {
	t.Helper()
	dir := t.TempDir()
	var version struct {
		Version string `json:"terraform_version"`
	}
	if json.Unmarshal(closureTerraform(t, dir, "version", "-json"), &version) != nil || version.Version != "1.5.7" {
		t.Fatal("requires Terraform1.5.7")
	}
	p, failure := loadPinned(operationalContext(t), f.s, f.objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	c := p.config
	regions := []string{}
	for r := range c.RegionMap {
		regions = append(regions, r)
	}
	destinations, routes := map[string]string{}, map[string]string{}
	for _, d := range c.Destinations {
		destinations[d.ID] = d.SecretID
	}
	for _, r := range c.Routes {
		routes[r.OwnerValue] = r.DestinationID
	}
	settings := map[string]any{"environment_name": c.EnvironmentName, "tenancy_ocid": c.TenancyID, "region": c.DeploymentRegion,
		"mysql_regions": regions, "mysql_compartments": c.CompartmentIDs, "ownership_kind": c.Ownership.Kind, "ownership_namespace": c.Ownership.Namespace, "ownership_key": c.Ownership.Key,
		"destinations": destinations, "routes": routes, "pagerduty_region": c.PagerDutyRegion, "mode": "PAUSED", "closure_confirm": true, "closure_request_number": 1, "closure_review_code": code}
	source, err := filepath.Abs("../../infra/modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	module := map[string]any{"source": source, "settings": settings, "catalog": map[string]any{"regions": p.catalog.Regions, "mysqlServices": p.catalog.MySQLServices},
		"installation_id": "b1-" + strings.Repeat("a", 64), "topics": map[string]string{"intake": c.IntakeTopicID, "operator": c.OperatorTopicID}}
	bundle := map[string]any{"module": map[string]any{"configuration": module}, "output": map[string]any{"configuration": map[string]string{"value": "${module.configuration.configuration_json}"}}}
	body, _ := json.Marshal(bundle)
	if err = os.WriteFile(filepath.Join(dir, "main.tf.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	closureTerraform(t, dir, "init", "-backend=false", "-input=false", "-no-color")
	closureTerraform(t, dir, "plan", "-refresh=false", "-input=false", "-no-color", "-out=result.tfplan")
	var plan struct {
		Planned struct {
			Outputs map[string]struct{ Value string }
		} `json:"planned_values"`
	}
	if json.Unmarshal(closureTerraform(t, dir, "show", "-json", "result.tfplan"), &plan) != nil {
		t.Fatal("invalid synthetic plan")
	}
	return []byte(plan.Planned.Outputs["configuration"].Value)
}
