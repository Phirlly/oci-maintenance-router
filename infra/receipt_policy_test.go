package infra_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The policy files each end their resource with the statements expression.
// Evaluate that complete expression, including conditional lists, in Terraform.
func policyStatements(t *testing.T, source string) string {
	t.Helper()
	match := regexp.MustCompile(`(?s)statements\s*=\s*(.*?)\n}`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatal("actual policy statements expression missing")
	}
	return match[1]
}

func TestConfirmationPoliciesGrantOnlyExactTestReads(t *testing.T) {
	for _, kind := range []string{"diagnostic", "operator"} {
		for _, mode := range []string{"TEST", "LIVE"} {
			for _, region := range []string{"iad", "phx"} {
				t.Run(kind+"/"+mode+"/"+region, func(t *testing.T) {
					source := readChecksumSource(t, "identity_"+kind+".tf")
					group, compartment, bucket := "ocid1.dynamicgroup.oc1.."+region, "ocid1.compartment.oc1.."+region, "bucket-"+region
					installation, request := "b1-"+strings.Repeat("a", 64), "request-1"
					config := `{"diagnostic":{"requestId":"request-1"},"operatorTest":{"requestId":"request-1"}}`
					quoted, _ := json.Marshal(config)
					expression := strings.NewReplacer(
						"oci_identity_dynamic_group.router.id", fmt.Sprintf("%q", group), "var.compartment_ocid", fmt.Sprintf("%q", compartment),
						"oci_objectstorage_bucket.state.name", fmt.Sprintf("%q", bucket), "local.installation_id", fmt.Sprintf("%q", installation),
						"local.catalog.regions[var.region]", fmt.Sprintf("%q", region), "module.configuration.configuration_json", string(quoted),
						"var.test_secret_ocid", `"ocid1.vaultsecret.oc1.iad.test"`, "local.receipt_confirmation", strconv.FormatBool(mode == "TEST"), "var.mode", strconv.Quote(mode), "\n", " ",
					).Replace(policyStatements(t, source))
					output, err := terraform(t, t.TempDir(), "jsonencode("+expression+")\n", "console", "-no-color")
					var encoded string
					var got []string
					if err != nil || json.Unmarshal(output, &encoded) != nil || json.Unmarshal([]byte(encoded), &got) != nil {
						t.Fatalf("policy evaluation: %s (%v)", output, err)
					}
					prefix, folder := "", "diagnostics"
					want := []string{}
					if kind == "operator" {
						folder = "operator-challenges"
						prefix = fmt.Sprintf("Allow dynamic-group id %s to use ons-topics in compartment id %s where all {target.compartment.id = '%s', request.region = '%s', ", group, compartment, compartment, region)
						want = append(want, prefix+"request.operation = 'GetTopic', request.permission = 'ONS_TOPIC_READ'}", prefix+"request.operation = 'GetSubscription', request.permission = 'ONS_TOPIC_SUBSCRIBE'}")
					}
					want = append(want, fmt.Sprintf("Allow dynamic-group id %s to read objects in compartment id %s where all {target.bucket.name = '%s', target.object.name = 'control/%s/%s/%s.json', request.permission = 'OBJECT_READ'}", group, compartment, bucket, folder, installation, request))
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("confirmation grants more than exact receipt reads: %q", got)
					}
				})
			}
		}
	}
}
