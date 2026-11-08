package infra_test

import (
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestFreeformAndNFCValuesKeepExactRoutingMeaning(t *testing.T) {
	settings := settingsFixture()
	settings["environment_name"] = "other-env"
	settings["tenancy_ocid"] = "ocid1.tenancy.oc1..other"
	settings["ownership_kind"] = "freeform"
	settings["ownership_namespace"] = ""
	settings["routes"] = map[string]string{"Équipe": "payments", " équipe ": "payments"}
	settings["pagerduty_region"] = "EU"
	got, output, err := render(t, settings)
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	var catalog configuration.Catalog
	if err := json.Unmarshal([]byte(got["catalog"]), &catalog); err != nil {
		t.Fatal(err)
	}
	config, err := configuration.Decode([]byte(got["configuration"]), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got["configuration"], "\"namespace\"") || config.EnvironmentName != "other-env" || config.PagerDutyRegion != "EU" {
		t.Fatal("freeform config contains namespace or ignores environment")
	}
	owners := map[string]bool{}
	for _, route := range config.Routes {
		owners[route.OwnerValue] = true
	}
	if !owners["Équipe"] || !owners[" équipe "] || len(owners) != 2 {
		t.Fatal("case or whitespace changed")
	}
}

// This proves a Terraform limitation, not support for decomposed customer tags.
// Native-form values must already be NFC; Go's exact matching stays unchanged.
func TestTerraformUnicodeBoundary(t *testing.T) {
	requireTerraform(t)
	expression := "jsonencode({composed = base64encode(\"é\"), decomposed = base64encode(\"e\\u0301\")})\n"
	output, err := terraform(t, t.TempDir(), expression, "console", "-no-color")
	var inner string
	if err != nil || json.Unmarshal(output, &inner) != nil {
		t.Fatalf("%v %s", err, output)
	}
	var got struct{ Composed, Decomposed string }
	if json.Unmarshal([]byte(inner), &got) != nil || got.Composed != "w6k=" || got.Decomposed != got.Composed {
		t.Fatalf("normalization boundary changed: %s", inner)
	}
	collision := "jsondecode(\"{\\\"é\\\":1,\\\"e\\\\u0301\\\":2}\")\n"
	output, err = terraform(t, t.TempDir(), collision, "console", "-no-color")
	if err == nil || !strings.Contains(string(output), "unsupported attribute") {
		t.Fatalf("JSON key collision behavior changed: %v %s", err, output)
	}
}
