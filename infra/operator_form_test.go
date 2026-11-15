package infra_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestOperatorFormExplainsConfirmationAndExplicitTest(t *testing.T) {
	f := readForm(t)
	email := f.Variables["operator_email"]
	if email.Type != "string" || email.Required || email.Default != "" || email.Visible == false {
		t.Fatal("operator email must be optional and visible in every mode, with no customer default")
	}
	for _, hint := range []string{"PREVIEW", "PAUSED", "confirm", "TEST", "Clear"} {
		if !strings.Contains(email.Description, hint) {
			t.Errorf("email setup lacks %s guidance", hint)
		}
	}
	run := f.Variables["operator_test_run"]
	visible := map[string]any{"and": []any{map[string]any{"eq": []any{"${mode}", "TEST"}}, map[string]any{"not": []any{map[string]any{"eq": []any{"${operator_email}", ""}}}}}}
	if run.Type != "integer" || !run.Required || run.Default != nil || run.Minimum != 1 || run.Maximum != 2147483647 || !reflect.DeepEqual(run.Visible, visible) {
		t.Fatal("operator test requires an explicit bounded run only when TEST and email are selected")
	}
	for _, hint := range []string{"unchanged", "new number", "email"} {
		if !strings.Contains(run.Description, hint) {
			t.Errorf("operator test lacks %s guidance", hint)
		}
	}
	network := f.Variables["existing_subnet_ocid"].Description
	for _, hint := range []string{"Notifications", "control", "data"} {
		if !strings.Contains(network, hint) {
			t.Errorf("existing network misses %s requirement", hint)
		}
	}
}
