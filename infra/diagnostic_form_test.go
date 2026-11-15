package infra_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeTestControlsRetainExplicitScopedIntent(t *testing.T) {
	form := readForm(t)
	mode := form.Variables["mode"]
	if mode.Type != "enum" || mode.Default != "PREVIEW" || !reflect.DeepEqual(mode.Enum, []string{"PREVIEW", "PAUSED", "TEST"}) {
		t.Fatal("mode must default to PREVIEW and exclude LIVE")
	}
	for _, name := range []string{"test_secret_compartment_ocid", "test_secret_ocid", "test_nonproduction", "test_run"} {
		v := form.Variables[name]
		if v.Required || v.Visible != true || !strings.Contains(v.Description, "TEST") {
			t.Fatalf("%s must stay optional and visible with TEST-only guidance", name)
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
	if !strings.Contains(mode.Description, "only in TEST") {
		t.Fatal("mode hint must explain when saved test settings are used")
	}
	secret := form.Variables["test_secret_ocid"]
	if secret.Type != "oci:kms:secret:id" || secret.DependsOn["compartmentId"] != "${test_secret_compartment_ocid}" || form.Variables["test_secret_compartment_ocid"].Default != "${secret_compartment_ocid}" {
		t.Fatal("test secret must use a separate scoped picker with a convenient compartment default")
	}
	run := form.Variables["test_run"]
	if run.Type != "integer" || run.Minimum != 0 || run.Maximum != 0 || run.Default != nil {
		t.Fatal("run must have no fallback or inactive range constraint; Terraform validates TEST bounds")
	}
	attestation := form.Variables["test_nonproduction"]
	if attestation.Type != "boolean" || attestation.Default != false {
		t.Fatal("test authority must not be preselected")
	}
}
