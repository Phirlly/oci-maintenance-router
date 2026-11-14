package infra_test

import (
	"reflect"
	"testing"
)

func TestNativeTestControlsRequireExplicitScopedIntent(t *testing.T) {
	form := readForm(t)
	mode := form.Variables["mode"]
	if mode.Type != "enum" || mode.Default != "PREVIEW" || !reflect.DeepEqual(mode.Enum, []string{"PREVIEW", "PAUSED", "TEST"}) {
		t.Fatal("mode must default to PREVIEW and exclude LIVE")
	}
	visible := map[string]any{"eq": []any{"${mode}", "TEST"}}
	for _, name := range []string{"test_secret_compartment_ocid", "test_secret_ocid", "test_nonproduction", "test_run"} {
		v := form.Variables[name]
		if !v.Required || !reflect.DeepEqual(v.Visible, visible) {
			t.Fatalf("%s must be required and visible only for TEST", name)
		}
	}
	secret := form.Variables["test_secret_ocid"]
	if secret.Type != "oci:kms:secret:id" || secret.DependsOn["compartmentId"] != "${test_secret_compartment_ocid}" || form.Variables["test_secret_compartment_ocid"].Default != "${secret_compartment_ocid}" {
		t.Fatal("test secret must use a separate scoped picker with a convenient compartment default")
	}
	run := form.Variables["test_run"]
	if run.Type != "integer" || run.Minimum != 1 || run.Maximum != 2147483647 || run.Default != nil {
		t.Fatal("test run must require explicit bounded intent, without fallback")
	}
	attestation := form.Variables["test_nonproduction"]
	if attestation.Type != "boolean" || attestation.Default != false {
		t.Fatal("test authority must not be preselected")
	}
}
