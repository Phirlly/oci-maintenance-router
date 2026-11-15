package infra_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestLivePolicyGrantsOnlySelectedResourcesAndState(t *testing.T) {
	source := readChecksumSource(t, "identity_live.tf")
	if !regexp.MustCompile(`compartment_id\s*=\s*var.tenancy_ocid`).MatchString(source) || !regexp.MustCompile(`provider\s*=\s*oci.home`).MatchString(source) {
		t.Fatal("cross-compartment scope requires home tenancy policy")
	}
	expression := strings.NewReplacer(
		"oci_identity_dynamic_group.router.id", `"group"`, "var.compartment_ocid", `"router-compartment"`, "oci_objectstorage_bucket.state.name", `"router-bucket"`,
		"var.mysql_compartments", `["ocid1.tenancy.oc1..root", "ocid1.compartment.oc1..selected"]`,
		"var.mysql_regions", `["us-ashburn-1", "us-phoenix-1"]`, "local.catalog.regions", `{"us-ashburn-1"="iad", "us-phoenix-1"="phx"}`,
		"var.destinations", `{"team-a"="ocid1.vaultsecret.oc1.iad.a", "team-b"="ocid1.vaultsecret.oc1.iad.b", "shared"="ocid1.vaultsecret.oc1.iad.a"}`, "\n", " ",
		"var.region", `"us-ashburn-1"`,
	).Replace(policyStatements(t, source))
	output, err := terraform(t, t.TempDir(), "jsonencode("+expression+")\n", "console", "-no-color")
	var encoded string
	var got []string
	if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &got) != nil {
		t.Fatalf("LIVE policy: %s (%v)", output, err)
	}
	object := "Allow dynamic-group id group to manage objects in compartment id router-compartment where all {target.bucket.name = 'router-bucket', "
	want := []string{
		object + "target.object.name = 'control/selected-baseline.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
		object + "target.object.name = 'review/preparation/*', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
		"Allow dynamic-group id group to read mysql-instances in tenancy where all {request.operation = 'GetDbSystem', request.permission = 'MYSQL_INSTANCE_READ', any {target.compartment.id = 'ocid1.tenancy.oc1..root', target.compartment.id = 'ocid1.compartment.oc1..selected'}, any {request.region = 'iad', request.region = 'phx'}}",
		"Allow dynamic-group id group to read secret-bundles in tenancy where all {request.permission = 'SECRET_BUNDLE_READ', any {target.secret.id = 'ocid1.vaultsecret.oc1.iad.a', target.secret.id = 'ocid1.vaultsecret.oc1.iad.b'}}",
		object + "target.object.name = 'control/operator-reporting.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}",
		"Allow dynamic-group id group to use ons-topics in compartment id router-compartment where all {target.compartment.id = 'router-compartment', request.region = 'iad', request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LIVE grants differ from exact selected scope: %q", got)
	}
	if !regexp.MustCompile(`depends_on\s*=\s*\[[^]]*oci_identity_policy.live`).MatchString(readChecksumSource(t, "deployment_check.tf")) {
		t.Fatal("activation must follow LIVE permissions")
	}
}

func TestLiveAndProofPolicyCountsFollowExplicitMode(t *testing.T) {
	for _, policy := range []string{"live", "diagnostic", "operator"} {
		source := readChecksumSource(t, "identity_"+policy+".tf")
		count := regexp.MustCompile(`(?m)^\s*count\s*=\s*(.*)$`).FindStringSubmatch(source)
		if len(count) != 2 {
			t.Fatal("missing conditional policy", policy)
		}
		for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
			for _, email := range []bool{false, true} {
				expression := strings.NewReplacer("var.mode", fmt.Sprintf("%q", mode), "local.operator_test_enabled", fmt.Sprint(mode == "TEST" && email)).Replace(count[1])
				got, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
				want := "0"
				if mode == "LIVE" || mode == "TEST" && (policy == "diagnostic" || policy == "operator" && email) {
					want = "1"
				}
				if err != nil || strings.TrimSpace(string(got)) != want {
					t.Fatalf("%s %s email=%v: %s (%v)", policy, mode, email, got, err)
				}
			}
		}
	}
}
