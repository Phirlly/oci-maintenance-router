package infra_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Actual pinned Terraform evaluates the real renderer over retained synthetic
// state. This proves request identity, not a managed Apply or PagerDuty receipt.
func TestDiagnosticRequestSurvivesEditsFailuresAndLegacyBundle(t *testing.T) {
	requireTerraform(t)
	data, err := os.ReadFile("configuration.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^\s*installation_id\s*=\s*(.*bucket_id.*)$`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatal("installation expression missing")
	}
	expression := strings.ReplaceAll(string(match[1]), "oci_objectstorage_bucket.state.bucket_id", "terraform_data.bucket.output")
	source, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeBundle := func(legacy bool) {
		t.Helper()
		bundle := map[string]any{
			"variable": map[string]any{"settings": map[string]string{"type": "any"}, "catalog": map[string]string{"type": "any"}, "bucket": map[string]string{"type": "string"}, "release": map[string]string{"type": "string"}},
			"resource": map[string]any{"terraform_data": map[string]any{"bucket": map[string]string{"input": "${var.bucket}"}, "release": map[string]string{"input": "${var.release}"}}},
			"output":   map[string]any{"request": map[string]string{"value": ""}},
		}
		if !legacy {
			bundle["locals"] = map[string]string{"installation_id": "${" + expression + "}"}
			bundle["module"] = map[string]any{"configuration": map[string]any{
				"source": source, "settings": "${var.settings}", "catalog": "${var.catalog}", "installation_id": "${local.installation_id}",
				"topics": map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"},
			}}
			bundle["output"] = map[string]any{"request": map[string]string{"value": `${try(jsondecode(module.configuration.configuration_json).diagnostic.requestId, "")}`}}
		}
		writeJSON(t, filepath.Join(dir, "main.tf.json"), bundle)
		if output, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
			t.Fatalf("init: %s (%v)", output, err)
		}
	}
	writeBundle(false)
	settings, catalog := diagnosticSettings(), catalogFixture()
	bucket, release := "ocid1.bucket.oc1.iad.retained", "release-one"
	first := ""
	for _, step := range []string{"initial", "repeat", "routes", "scope", "release", "catalog", "secret", "PD region", "failed apply", "retry", "missing run", "null run", "restore run", "paused", "preview", "test again", "legacy omission", "reupgrade missing", "reupgrade retained", "new run", "return to earlier run", "new bucket"} {
		t.Run(step, func(t *testing.T) {
			switch step {
			case "routes":
				settings["routes"] = map[string]string{"another-owner": "payments"}
			case "scope":
				settings["mysql_regions"] = []string{"us-ashburn-1", "us-phoenix-1"}
			case "release":
				release = "release-two"
			case "catalog":
				catalog["regions"] = map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx", "us-sanjose-1": "sjc"}
			case "secret":
				settings["test_secret_ocid"] = "ocid1.vaultsecret.oc1.iad.different"
			case "PD region":
				settings["pagerduty_region"] = "EU"
			case "failed apply":
				settings["test_nonproduction"] = false
			case "retry":
				settings["test_nonproduction"] = true
			case "missing run", "legacy omission":
				delete(settings, "test_run")
				if step == "legacy omission" {
					writeBundle(true)
				}
			case "null run":
				settings["test_run"] = nil
			case "restore run", "reupgrade retained", "return to earlier run":
				settings["test_run"] = 5
			case "paused":
				settings["mode"] = "PAUSED"
			case "preview":
				settings["mode"] = "PREVIEW"
			case "test again":
				settings["mode"] = "TEST"
			case "reupgrade missing":
				writeBundle(false)
			case "new run":
				settings["test_run"] = 6
			case "new bucket":
				bucket = "ocid1.bucket.oc1.iad.replacement"
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": settings, "catalog": catalog, "bucket": bucket, "release": release})
			output, err := terraform(t, dir, "", "apply", "-auto-approve", "-input=false", "-no-color")
			if step == "failed apply" || step == "missing run" || step == "null run" || step == "reupgrade missing" {
				if err == nil || !strings.Contains(string(output), "precondition failed") {
					t.Fatalf("lost intent must fail before authorizing any request: %s (%v)", output, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply: %s (%v)", output, err)
			}
			output, err = terraform(t, dir, "", "output", "-json", "request")
			var got string
			if err != nil || json.Unmarshal(output, &got) != nil {
				t.Fatalf("request: %s (%v)", output, err)
			}
			if first == "" {
				first = got
				if !regexp.MustCompile(`^d1-[a-f0-9]{64}$`).MatchString(first) {
					t.Fatal("invalid initial test request")
				}
			}
			switch step {
			case "paused", "preview", "legacy omission":
				if got != "" {
					t.Fatal("inactive/legacy configuration authorized a test")
				}
			case "new run", "new bucket":
				if got == first || got == "" {
					t.Fatal("explicit new run or installation did not get a distinct request")
				}
			default:
				if got != first {
					t.Fatalf("implicit fresh request on %s: %s != %s", step, got, first)
				}
			}
		})
	}
}
