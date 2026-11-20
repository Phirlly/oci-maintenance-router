package infra_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func setupPolicy(t *testing.T, name, mode string, confirming bool) []string {
	t.Helper()
	source := readChecksumSource(t, "identity_"+name+".tf")
	config := `{"diagnostic":{"requestId":"sd1-attempt"},"operatorTest":{"requestId":"so1-attempt"}}`
	expression := strings.NewReplacer(
		"oci_identity_dynamic_group.router.id", `"group"`, "var.compartment_ocid", `"compartment"`,
		"oci_objectstorage_bucket.state.name", `"bucket"`, "local.installation_id", `"installation"`,
		"local.catalog.regions[var.region]", `"iad"`, "module.configuration.configuration_json", strconv.Quote(config),
		"var.test_secret_ocid", `"secret"`, "local.receipt_confirmation", strconv.FormatBool(confirming),
		"local.diagnostic_request_id", `"sd1-attempt"`, "local.operator_request_id", `"so1-attempt"`,
		"local.setup_paired", strconv.FormatBool(mode == "TEST"), "local.mode", strconv.Quote(mode), "var.mode", strconv.Quote(mode), "\n", " ",
	).Replace(policyStatements(t, source))
	out, err := terraform(t, t.TempDir(), "jsonencode("+expression+")\n", "console", "-no-color")
	var inner string
	var result []string
	if err != nil || json.Unmarshal(out, &inner) != nil || json.Unmarshal([]byte(inner), &result) != nil {
		t.Fatalf("policy: %s (%v)", out, err)
	}
	return result
}

func objectPermission(verb, path, permission string) string {
	return fmt.Sprintf("Allow dynamic-group id group to %s objects in compartment id compartment where all {target.bucket.name = 'bucket', target.object.name = '%s', %s}", verb, path, permission)
}

func TestSetupHeadPermissionsFollowActualTransitions(t *testing.T) {
	read := "request.permission = 'OBJECT_READ'"
	for _, tc := range []struct {
		mode       string
		confirming bool
		want       []string
	}{
		{"TEST", false, []string{objectPermission("manage", "control/setup-current/installation.json", "any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}")}},
		{"TEST", true, []string{objectPermission("manage", "control/setup-current/installation.json", "any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_OVERWRITE'}"), objectPermission("manage", "control/setup-confirmations/installation/*", "any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE'}")}},
		{"LIVE", false, []string{objectPermission("read", "control/setup-current/installation.json", read), objectPermission("read", "control/setup-confirmations/installation/*", read)}},
	} {
		t.Run(fmt.Sprint(tc.mode, tc.confirming), func(t *testing.T) {
			if got := setupPolicy(t, "setup", tc.mode, tc.confirming); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("setup authority differs: %q", got)
			}
		})
	}
}

func TestSavedReadinessReadsInstallationProofsWithoutSecretOrWrites(t *testing.T) {
	for _, name := range []string{"diagnostic", "operator"} {
		got := setupPolicy(t, name, "LIVE", false)
		folder, count := "diagnostics", 1
		if name == "operator" {
			folder, count = "operator-challenges", 3
		}
		want := objectPermission("read", "control/"+folder+"/installation/*", "request.permission = 'OBJECT_READ'")
		if len(got) != count || got[len(got)-1] != want {
			t.Fatalf("%s LIVE proof scope: %q", name, got)
		}
		for _, statement := range got {
			for _, forbidden := range []string{"SECRET_BUNDLE_READ", "OBJECT_CREATE", "OBJECT_OVERWRITE", "OBJECT_DELETE", "PublishMessage"} {
				if strings.Contains(statement, forbidden) {
					t.Fatalf("LIVE setup proof policy granted %s", forbidden)
				}
			}
		}
	}
}
