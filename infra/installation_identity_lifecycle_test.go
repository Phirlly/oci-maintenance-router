package infra_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Retain the real identity expression and configuration module. The synthetic
// bucket stands for the existing protected OCI resource; this is not live rollback.
func TestInstallationIdentitySurvivesPackageRollback(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("configuration.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^\s*installation_id\s*=\s*(.*bucket_id.*)$`).FindSubmatch(source)
	if len(match) != 2 || !regexp.MustCompile(`installation_id\s*=\s*local.installation_id`).Match(source) {
		t.Fatal("configuration must use the protected bucket's unique identity")
	}
	expression := strings.ReplaceAll(string(match[1]), "oci_objectstorage_bucket.state.bucket_id", "terraform_data.bucket.output")
	module, err := filepath.Abs("modules/configuration")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeBundle := func(operational bool) {
		t.Helper()
		configuration := `${jsonencode({schemaVersion=1,mode="PREVIEW"})}`
		bundle := map[string]any{
			"variable": map[string]any{
				"settings": map[string]string{"type": "any"}, "bucket_ocid": map[string]string{"type": "string"}, "release": map[string]string{"type": "string"},
			},
			"output": map[string]any{
				"installation":    map[string]string{"value": `${try(jsondecode(terraform_data.desired.output.configuration).installationId, "")}`},
				"bucket_resource": map[string]string{"value": "${terraform_data.bucket.id}"},
			},
		}
		if operational {
			configuration = "${module.configuration.configuration_json}"
			bundle["locals"] = map[string]string{"installation_id": "${" + expression + "}"}
			bundle["module"] = map[string]any{"configuration": map[string]any{
				"source": module, "settings": "${var.settings}", "catalog": catalogFixture(), "installation_id": "${local.installation_id}",
				"topics": map[string]string{"intake": "ocid1.onstopic.oc1.iad.intake", "operator": "ocid1.onstopic.oc1.iad.operator"},
			}}
		}
		bundle["resource"] = map[string]any{"terraform_data": map[string]any{
			"bucket":  map[string]string{"input": "${var.bucket_ocid}"},
			"desired": map[string]any{"input": map[string]string{"configuration": configuration, "release": "${var.release}"}},
		}}
		writeJSON(t, filepath.Join(dir, "main.tf.json"), bundle)
		if output, err := terraform(t, dir, "", "init", "-backend=false", "-input=false", "-no-color"); err != nil {
			t.Fatalf("init: %s (%v)", output, err)
		}
	}
	writeBundle(true)
	bucket := "ocid1.bucket.oc1.iad.retained-example"
	want := fmt.Sprintf("b1-%x", sha256.Sum256([]byte(bucket)))
	settings := settingsFixture()
	release := "first-release"
	retainedResource := ""
	for _, step := range []string{"initial", "unchanged", "routing", "scope", "release", "legacy bundle", "re-upgrade", "different bucket"} {
		t.Run(step, func(t *testing.T) {
			switch step {
			case "routing":
				settings["routes"] = map[string]string{"changed-team": "payments"}
			case "scope":
				settings["mysql_regions"] = []string{"us-phoenix-1", "us-ashburn-1"}
			case "release":
				release = "second-release"
			case "legacy bundle":
				writeBundle(false) // The older bundle has no installation local/module input.
			case "re-upgrade":
				writeBundle(true)
			case "different bucket":
				bucket = "ocid1.bucket.oc1.iad.different-example"
			}
			writeJSON(t, filepath.Join(dir, "terraform.tfvars.json"), map[string]any{"settings": settings, "bucket_ocid": bucket, "release": release})
			output, err := terraform(t, dir, "", "apply", "-auto-approve", "-input=false", "-no-color")
			if err != nil {
				t.Fatalf("apply: %s (%v)", output, err)
			}
			output, err = terraform(t, dir, "", "output", "-raw", "installation")
			if err != nil {
				t.Fatalf("identity: %s (%v)", output, err)
			}
			got := string(output)
			switch step {
			case "legacy bundle":
				if got != "" {
					t.Fatal("legacy fixture must omit installation identity")
				}
			case "different bucket":
				if got == want || got != fmt.Sprintf("b1-%x", sha256.Sum256([]byte(bucket))) {
					t.Fatal("a different bucket must not impersonate retained installation history")
				}
			default:
				if got != want {
					t.Fatalf("installation identity changed: %q, want %q", got, want)
				}
			}
			output, err = terraform(t, dir, "", "output", "-raw", "bucket_resource")
			if err != nil {
				t.Fatalf("bucket state: %s (%v)", output, err)
			}
			if retainedResource == "" {
				retainedResource = string(output)
			} else if string(output) != retainedResource {
				t.Fatal("fixture unexpectedly replaced the retained bucket resource")
			}
		})
	}
}
