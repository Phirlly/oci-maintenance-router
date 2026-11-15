package infra_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestAutomationPoliciesRestrictExactPrincipalsAndOperations(t *testing.T) {
	source := readChecksumSource(t, "identity_automation.tf")
	group := checksumBlock(source, `resource "oci_identity_dynamic_group" "schedule"`)
	if !strings.Contains(group, "resource.type = 'resourceschedule'") || !strings.Contains(group, "resource.id = '${oci_resource_scheduler_schedule.reconcile.id}'") {
		t.Fatal("schedule group is not exact")
	}
	for _, name := range []string{"schedule", "detached_failure"} {
		block := checksumBlock(source, `resource "oci_identity_policy" "`+name+`"`)
		match := regexp.MustCompile(`(?s)statements\s*=\s*(\[.*\])`).FindStringSubmatch(block)
		if len(match) != 2 {
			t.Fatal("missing policy", name)
		}
		expression := strings.NewReplacer("oci_identity_dynamic_group.schedule.id", `"group"`, "oci_functions_function.router.id", `"function"`, "oci_functions_application.router.id", `"application"`, "var.compartment_ocid", `"compartment"`, "local.catalog.regions[var.region]", `"iad"`, "\n", " ").Replace(match[1])
		output, err := terraform(t, t.TempDir(), "jsonencode("+expression+")\n", "console", "-no-color")
		var encoded string
		var statements []string
		if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &statements) != nil {
			t.Fatal("invalid policy", name, string(output), err)
		}
		want := []string{
			"Allow dynamic-group id group to use fn-invocation in compartment id compartment where all {target.function.id = 'function', request.region = 'iad', request.operation = 'InvokeFunction', request.permission = 'FN_INVOCATION'}",
			"Allow dynamic-group id group to read fn-function in compartment id compartment where all {target.function.id = 'function', request.region = 'iad', request.operation = 'GetFunction', request.permission = 'FN_FUNCTION_READ'}",
		}
		if name == "detached_failure" {
			want = []string{"Allow any-user to use ons-topics in compartment id compartment where all {request.principal.type = 'fnapp', request.principal.id = 'application', request.principal.compartment.id = 'compartment', request.region = 'iad', request.operation = 'PublishMessage', request.permission = 'ONS_TOPIC_PUBLISH'}"}
		}
		if !slices.Equal(statements, want) {
			t.Fatalf("policy widened or changed:\n%v\nwant %v", statements, want)
		}
		if name == "schedule" && strings.Contains(block, "var.mode") {
			t.Fatal("schedule invocation IAM must exist before later activation")
		}
	}
	check := readChecksumSource(t, "deployment_check.tf")
	if strings.Contains(check, "oci_identity_policy.schedule") || strings.Contains(check, "oci_identity_dynamic_group.schedule") {
		t.Fatal("schedule IAM introduces activation cycle")
	}
}

func TestDetachedDestinationIsExplicitAndLiveOnly(t *testing.T) {
	block := checksumBlock(readChecksumSource(t, "function.tf"), `resource "oci_functions_function" "router"`)
	if !regexp.MustCompile(`detached_mode_timeout_in_seconds\s*=\s*300`).MatchString(block) || !strings.Contains(block, "oci_identity_policy.detached_failure") {
		t.Fatal("missing bounded timeout or publication ordering")
	}
	failure := checksumBlock(block, "failure_destination")
	success := checksumBlock(block, "success_destination")
	if !regexp.MustCompile(`kind\s*=\s*"NONE"`).MatchString(success) {
		t.Fatal("success destination must remain disabled")
	}
	kind := regexp.MustCompile(`(?m)^\s*kind\s*=\s*(.+)$`).FindStringSubmatch(failure)
	topic := regexp.MustCompile(`(?m)^\s*topic_id\s*=\s*(.+)$`).FindStringSubmatch(failure)
	policy := checksumBlock(readChecksumSource(t, "identity_automation.tf"), `resource "oci_identity_policy" "detached_failure"`)
	count := regexp.MustCompile(`(?m)^\s*count\s*=\s*(.+)$`).FindStringSubmatch(policy)
	if len(kind) != 2 || len(topic) != 2 || len(count) != 2 {
		t.Fatal("missing explicit failure lifecycle")
	}
	for _, mode := range []string{"PREVIEW", "TEST", "LIVE", "PAUSED"} {
		replace := strings.NewReplacer("var.mode", fmt.Sprintf("%q", mode), "oci_ons_notification_topic.operator.id", `"operator"`)
		out, err := terraform(t, t.TempDir(), "jsonencode(["+replace.Replace(kind[1]+","+topic[1]+","+count[1])+"])\n", "console", "-no-color")
		var encoded string
		if err != nil || json.Unmarshal(out, &encoded) != nil {
			t.Fatal(string(out), err)
		}
		want := `["NONE",null,0]`
		if mode == "LIVE" {
			want = `["NOTIFICATION","operator",1]`
		}
		if encoded != want {
			t.Fatal("destination not cleared or wrong authority", mode, encoded)
		}
	}
}
