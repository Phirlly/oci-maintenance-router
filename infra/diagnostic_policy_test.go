package infra_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestDiagnosticIAMGrantsOnlyConfiguredTestAuthority(t *testing.T) {
	requireTerraform(t)
	data, err := os.ReadFile("identity_diagnostic.tf")
	if err != nil {
		t.Fatal(err)
	}
	block := checksumBlock(string(data), `resource "oci_identity_policy" "diagnostic"`)
	for _, pattern := range []string{`provider\s*=\s*oci.home`, `compartment_id\s*=\s*var.tenancy_ocid`} {
		if !regexp.MustCompile(pattern).MatchString(block) {
			t.Fatalf("diagnostic policy must be tenancy/home placed: %s", pattern)
		}
	}
	statements := policyStatements(t, block)
	for _, name := range []string{"alpha", "beta"} {
		settings := diagnosticSettings()
		settings["test_run"] = 2
		settings["test_secret_ocid"] = "ocid1.vaultsecret.oc1.iad." + name
		values, message, err := render(t, settings)
		if err != nil {
			t.Fatalf("render: %s (%v)", message, err)
		}
		config := renderedConfig(t, values)
		group, compartment, bucket := "ocid1.dynamicgroup.oc1.."+name, "ocid1.compartment.oc1.."+name, "router-"+name
		dir := t.TempDir()
		writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{"locals": map[string]string{
			"group": group, "compartment": compartment, "bucket": bucket, "secret": config.Diagnostic.SecretID,
			"installation_id": config.InstallationID, "configuration": values["configuration"],
		}})
		expression := strings.NewReplacer("var.mode", `"TEST"`, "local.receipt_confirmation", "false", "oci_identity_dynamic_group.router.id", "local.group", "var.compartment_ocid", "local.compartment",
			"oci_objectstorage_bucket.state.name", "local.bucket", "var.test_secret_ocid", "local.secret",
			"module.configuration.configuration_json", "local.configuration", "\n", " ").Replace(statements)
		output, err := terraform(t, dir, "jsonencode("+expression+")\n", "console", "-no-color")
		var encoded string
		var got []string
		if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &got) != nil {
			t.Fatalf("policy evaluation: %s (%v)", output, err)
		}
		want := []string{
			fmt.Sprintf("Allow dynamic-group id %s to read secret-bundles in tenancy where all {target.secret.id = '%s', request.permission = 'SECRET_BUNDLE_READ'}", group, config.Diagnostic.SecretID),
			fmt.Sprintf("Allow dynamic-group id %s to manage objects in compartment id %s where all {target.bucket.name = '%s', target.object.name = 'control/diagnostics/%s/%s.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}", group, compartment, bucket, config.InstallationID, config.Diagnostic.RequestID),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("test IAM scope differs: %q, want %q", got, want)
		}
	}
	invoke, err := os.ReadFile("deployment_check.tf")
	if err != nil || !regexp.MustCompile(`depends_on\s*=\s*\[[^]]*oci_identity_policy.diagnostic`).Match(invoke) {
		t.Fatal("invocation must follow conditional test permissions", err)
	}
}
