package infra_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestExistingSubnetPredicate(t *testing.T) {
	requireTerraform(t)
	data, err := os.ReadFile("network.tf")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?s)data\s+"oci_core_subnet"\s+"existing"\s*\{.*?postcondition\s*\{\s*condition\s*=\s*(.*?)\s*error_message\s*=`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatal("cannot locate actual existing-subnet postcondition")
	}
	for _, tc := range []struct {
		name, ad, private, state, vcn, want string
	}{
		{"regional null", `null`, "true", `"AVAILABLE"`, `"selected-vcn"`, "true"},
		{"regional empty", `""`, "true", `"AVAILABLE"`, `"selected-vcn"`, "true"},
		{"named AD", `"example:US-ASHBURN-AD-1"`, "true", `"AVAILABLE"`, `"selected-vcn"`, "false"},
		{"whitespace AD", `" "`, "true", `"AVAILABLE"`, `"selected-vcn"`, "false"},
		{"public null", `null`, "false", `"AVAILABLE"`, `"selected-vcn"`, "false"},
		{"public empty", `""`, "false", `"AVAILABLE"`, `"selected-vcn"`, "false"},
		{"creating null", `null`, "true", `"CREATING"`, `"selected-vcn"`, "false"},
		{"terminated null", `null`, "true", `"TERMINATED"`, `"selected-vcn"`, "false"},
		{"creating empty", `""`, "true", `"CREATING"`, `"selected-vcn"`, "false"},
		{"wrong VCN null", `null`, "true", `"AVAILABLE"`, `"different-vcn"`, "false"},
		{"wrong VCN empty", `""`, "true", `"AVAILABLE"`, `"different-vcn"`, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expression := strings.NewReplacer(
				"self.availability_domain", tc.ad,
				"self.prohibit_public_ip_on_vnic", tc.private,
				"self.state", tc.state,
				"self.vcn_id", tc.vcn,
				"var.existing_vcn_ocid", `"selected-vcn"`,
				"\n", " ",
			).Replace(string(match[1]))
			output, err := terraform(t, t.TempDir(), expression+"\n", "console", "-no-color")
			if err != nil || strings.TrimSpace(string(output)) != tc.want {
				t.Fatalf("existing-subnet predicate got %s (%v), want %s", output, err, tc.want)
			}
		})
	}
}
