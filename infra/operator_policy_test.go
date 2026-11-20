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

func TestOperatorPolicyLimitsOperationsRegionAndRecord(t *testing.T) {
	requireTerraform(t)
	data, err := os.ReadFile("identity_operator.tf")
	if err != nil {
		t.Fatal(err)
	}
	block := checksumBlock(string(data), `resource "oci_identity_policy" "operator"`)
	for _, pattern := range []string{`provider\s*=\s*oci.home`, `compartment_id\s*=\s*var.compartment_ocid`} {
		if !regexp.MustCompile(pattern).MatchString(block) {
			t.Fatalf("incorrect conditional policy boundary: %s", pattern)
		}
	}
	statements := policyStatements(t, block)
	for _, region := range []string{"iad", "phx"} {
		group, compartment, bucket := "ocid1.dynamicgroup.oc1.."+region, "ocid1.compartment.oc1.."+region, "router-"+region
		installation, request := "b1-"+strings.Repeat("a", 64), "o1-"+strings.Repeat("b", 64)
		dir := t.TempDir()
		writeJSON(t, filepath.Join(dir, "main.tf.json"), map[string]any{"locals": map[string]any{"group": group, "compartment": compartment, "bucket": bucket, "installation_id": installation, "region_key": region, "configuration": `{"operatorTest":{"requestId":"` + request + `"}}`}})
		expression := strings.NewReplacer("local.operator_request_id", fmt.Sprintf("%q", request), "local.mode", `"TEST"`, "local.receipt_confirmation", "false", "oci_identity_dynamic_group.router.id", "local.group", "var.compartment_ocid", "local.compartment", "oci_objectstorage_bucket.state.name", "local.bucket", "local.catalog.regions[var.region]", "local.region_key", "module.configuration.configuration_json", "local.configuration", "\n", " ").Replace(statements)
		output, err := terraform(t, dir, "jsonencode("+expression+")\n", "console", "-no-color")
		var encoded string
		var got []string
		if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &got) != nil {
			t.Fatalf("policy: %s (%v)", output, err)
		}
		prefix := fmt.Sprintf("Allow dynamic-group id %s to use ons-topics in compartment id %s where all {target.compartment.id = '%s', request.region = '%s', ", group, compartment, compartment, region)
		want := []string{
			prefix + "request.operation = 'GetTopic', request.permission = 'ONS_TOPIC_READ'}",
			prefix + "request.operation = 'GetSubscription', request.permission = 'ONS_TOPIC_SUBSCRIBE'}",
			prefix + "request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}",
			fmt.Sprintf("Allow dynamic-group id %s to manage objects in compartment id %s where all {target.bucket.name = '%s', target.object.name = 'control/operator-challenges/%s/%s.json', any {request.permission = 'OBJECT_READ', request.permission = 'OBJECT_CREATE', request.permission = 'OBJECT_OVERWRITE'}}", group, compartment, bucket, installation, request),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("operator permissions differ: %q", got)
		}
	}
	invoke, err := os.ReadFile("deployment_check.tf")
	if err != nil || !regexp.MustCompile(`depends_on\s*=\s*\[[^]]*oci_identity_policy.operator`).Match(invoke) {
		t.Fatal("invocation must follow operator IAM", err)
	}
}

func TestOperatorSubscriptionIsOwnedAndIndependentOfMode(t *testing.T) {
	source := readChecksumSource(t, "operator.tf")
	block := checksumBlock(source, `resource "oci_ons_subscription" "operator"`)
	for _, pattern := range []string{`count\s*=\s*var.operator_email != "" \? 1 : 0`, `compartment_id\s*=\s*var.compartment_ocid`, `topic_id\s*=\s*oci_ons_notification_topic.operator.id`, `protocol\s*=\s*"EMAIL"`, `endpoint\s*=\s*var.operator_email`} {
		if !regexp.MustCompile(pattern).MatchString(block) {
			t.Fatalf("subscription ownership differs: %s", pattern)
		}
	}
	if strings.Contains(block, "local.mode") {
		t.Fatal("mode change must not replace the subscription")
	}
	configuration := readChecksumSource(t, "configuration.tf")
	if !regexp.MustCompile(`operator_subscription_id\s*=\s*try\(oci_ons_subscription.operator\[0\].id, ""\)`).MatchString(configuration) {
		t.Fatal("configuration must use deployment-owned subscription")
	}
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		for _, email := range []string{"", "ops@example.invalid"} {
			match := regexp.MustCompile(`(?m)^\s*operator_test_enabled\s*=\s*(.*)$`).FindStringSubmatch(source)
			if len(match) != 2 {
				t.Fatal("missing operator authority expression")
			}
			expression := strings.NewReplacer("local.mode", fmt.Sprintf("%q", mode), "var.operator_email", fmt.Sprintf("%q", email)).Replace(match[1])
			output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
			if err != nil || strings.TrimSpace(string(output)) != fmt.Sprint(mode == "TEST" && email != "") {
				t.Fatalf("operator gating: %s (%v)", output, err)
			}
		}
	}
}
