package infra_test

import (
	"strings"
	"testing"
)

func TestNativeTestControlsRetainExplicitScopedIntent(t *testing.T) {
	form := readForm(t)
	for _, name := range []string{"test_secret_compartment_ocid", "test_secret_ocid", "test_nonproduction", "setup_attempt"} {
		v := form.Variables[name]
		if v.Required || v.Description == "" {
			t.Fatalf("%s needs guidance and action-dependent Terraform validation", name)
		}
	}
	for _, group := range form.VariableGroups {
		for _, field := range group.Variables {
			if strings.HasPrefix(field, "${test_") || strings.HasPrefix(field, "${operator_test_") || field == "${operator_receipt_code}" {
				if group.Visible != nil && group.Visible != true {
					t.Fatalf("test settings hidden by group %s", group.Title)
				}
			}
		}
	}
	if !strings.Contains(form.Variables["routing_state"].Description, "confirming both tests") {
		t.Fatal("routing hint must explain activation requirements")
	}
	secret := form.Variables["test_secret_ocid"]
	if secret.Type != "oci:kms:secret:id" || secret.DependsOn["compartmentId"] != "${test_secret_compartment_ocid}" {
		t.Fatal("test secret must use a separate scoped picker")
	}
	compartment := form.Variables["test_secret_compartment_ocid"]
	if compartment.Type != "oci:identity:compartment:id" || compartment.Default != nil {
		t.Fatal("optional test compartment must start unselected without a literal or expression default")
	}
	run := form.Variables["setup_attempt"]
	if run.Type != "integer" || run.Minimum != 0 || run.Maximum != 0 || run.Default != nil {
		t.Fatal("run must have no fallback or inactive range constraint; Terraform validates TEST bounds")
	}
	attestation := form.Variables["test_nonproduction"]
	if attestation.Type != "boolean" || attestation.Default != false {
		t.Fatal("test authority must not be preselected")
	}
}
