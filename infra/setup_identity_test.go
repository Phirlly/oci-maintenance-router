package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func setupIdentity(t *testing.T, values map[string]string, attempt int) map[string]string {
	t.Helper()
	c := renderedConfig(t, values)
	source := strings.Split(readChecksumSource(t, "identity_setup.tf"), `resource "oci_identity_policy" "setup"`)[0]
	source = strings.NewReplacer(
		"var.tenancy_ocid", strconv.Quote(c.TenancyID), "var.region", strconv.Quote(c.DeploymentRegion),
		"oci_objectstorage_bucket.state.namespace", `"synthetic-namespace"`, "oci_objectstorage_bucket.state.name", `"synthetic-bucket"`,
		"local.installation_id", strconv.Quote(c.InstallationID), "var.setup_attempt", strconv.Itoa(attempt),
		"local.diagnostic_only", strconv.FormatBool(c.SchemaVersion == 7), "module.configuration.configuration_json", strconv.Quote(values["configuration"]),
	).Replace(source)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := terraform(t, dir, "jsonencode({environment=local.setup_environment_id,diagnostic=local.diagnostic_request_id,operator=local.operator_request_id})\n", "console", "-no-color")
	var encoded string
	var result map[string]string
	if err != nil || json.Unmarshal(out, &encoded) != nil || json.Unmarshal([]byte(encoded), &result) != nil {
		t.Fatalf("identity evaluation: %s (%v)", out, err)
	}
	return result
}

func TestSetupIAMIdentityMatchesRuntimeEnvironmentAndAttemptContract(t *testing.T) {
	for _, attempt := range []int{1, 2147483647} {
		settings := setupSettings("TEST", "SEND_TEST", true)
		settings["setup_attempt"] = attempt
		values, message, err := renderSetup(t, settings)
		if err != nil {
			t.Fatalf("render: %s (%v)", message, err)
		}
		c := renderedConfig(t, values)
		environment, err := c.EnvironmentID(configuration.LedgerLocation{Namespace: "synthetic-namespace", Bucket: "synthetic-bucket"})
		if err != nil {
			t.Fatal(err)
		}
		got := setupIdentity(t, values, attempt)
		if got["environment"] != environment {
			t.Fatal("Terraform IAM environment differs from decoded runtime identity")
		}
		for _, channel := range []struct{ name, domain, prefix string }{{"diagnostic", "setup-diagnostic-v1", "sd1-"}, {"operator", "setup-operator-v1", "so1-"}} {
			encoded, _ := json.Marshal([]any{channel.domain, environment, c.InstallationID, attempt})
			digest := sha256.Sum256(encoded)
			if got[channel.name] != channel.prefix+hex.EncodeToString(digest[:]) {
				t.Fatal("IAM cannot access the runtime's selected attempt", channel.name)
			}
		}
	}
}

func TestSetupAndLegacyControlsPreserveRoutingIdentity(t *testing.T) {
	legacy, message, err := renderSetup(t, operatorSettings())
	if err != nil {
		t.Fatalf("legacy: %s (%v)", message, err)
	}
	base := renderedConfig(t, legacy)
	want, _ := base.RoutingGeneration()
	for _, paired := range []bool{false, true} {
		for _, action := range []string{"SEND_TEST", "CONFIRM", "NONE"} {
			mode := "TEST"
			if action == "NONE" {
				mode = "PREVIEW"
			}
			values, message, err := renderSetup(t, setupSettings(mode, action, paired))
			if err != nil {
				t.Fatalf("render: %s (%v)", message, err)
			}
			c := renderedConfig(t, values)
			got, _ := c.RoutingGeneration()
			if got != want || c.InstallationID != base.InstallationID {
				t.Fatal("setup migration changed routing or retained installation")
			}
			if !paired && action != "NONE" {
				ids := setupIdentity(t, values, 1)
				if ids["diagnostic"] != c.Diagnostic.RequestID || c.Diagnostic.RequestID == base.Diagnostic.RequestID {
					t.Fatal("diagnostic-only IAM or legacy separation failed")
				}
			}
		}
	}
}
