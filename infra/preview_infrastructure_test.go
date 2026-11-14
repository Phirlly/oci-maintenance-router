package infra_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestPreviewAnnouncementPolicyAndInvocationDependencies(t *testing.T) {
	requireTerraform(t)
	source, err := os.ReadFile("identity.tf")
	if err != nil {
		t.Fatal(err)
	}
	block := checksumBlock(string(source), `resource "oci_identity_policy" "announcements"`)
	for _, pattern := range []string{`provider\s*=\s*oci.home`, `compartment_id\s*=\s*var.tenancy_ocid`} {
		if !regexp.MustCompile(pattern).MatchString(block) {
			t.Fatal("announcement policy must use home-region tenancy placement")
		}
	}
	match := regexp.MustCompile(`(?s)statements\s*=\s*(\[.*?\])`).FindStringSubmatch(block)
	if len(match) != 2 {
		t.Fatal("missing announcement read policy")
	}
	expression := strings.ReplaceAll(match[1], "${oci_identity_dynamic_group.router.id}", "ocid1.dynamicgroup.oc1..example")
	output, err := terraform(t, t.TempDir(), "jsonencode("+strings.ReplaceAll(expression, "\n", " ")+")\n", "console", "-no-color")
	var encoded string
	var statements []string
	if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &statements) != nil ||
		len(statements) != 1 || statements[0] != "Allow dynamic-group id ocid1.dynamicgroup.oc1..example to read announcements in tenancy" {
		t.Fatalf("announcement grant differs from read-only contract: %s (%v)", output, err)
	}
	invoke, err := os.ReadFile("deployment_check.tf")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := regexp.MustCompile(`depends_on\s*=\s*\[([^]]*)\]`).FindSubmatch(invoke)
	if len(dependencies) != 2 {
		t.Fatal("missing invocation dependencies")
	}
	for _, dependency := range []string{"oci_identity_policy.router", "oci_identity_policy.announcements", "data.oci_functions_function.archive"} {
		if !strings.Contains(string(dependencies[1]), dependency) {
			t.Fatalf("PREVIEW invocation must follow %s", dependency)
		}
	}
}

func TestPreviewExecutionBudgetAndPrivateNetwork(t *testing.T) {
	function, err := os.ReadFile("function.tf")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`timeout_in_seconds\s*=\s*300\b`).Match(function) {
		t.Fatal("synchronous execution must fit runtime work, loading and cleanup")
	}
	network, err := os.ReadFile("network.tf")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(network), `resource "oci_core_internet_gateway"`) {
		t.Fatal("router subnet must remain private")
	}
	form := readForm(t)
	for _, name := range []string{"installation_id", "preparation", "test_request_id", "recovery"} {
		if _, present := form.Variables[name]; present {
			t.Fatalf("internal PREVIEW control exposed to customer: %s", name)
		}
	}
}
