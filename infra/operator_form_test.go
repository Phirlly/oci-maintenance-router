package infra_test

import (
	"strings"
	"testing"
)

func TestOperatorFormExplainsConfirmationAndExplicitTest(t *testing.T) {
	f := readForm(t)
	email := f.Variables["operator_email"]
	if email.Type != "string" || email.Required || email.Default != "" || email.Visible == false {
		t.Fatal("operator email must be optional and visible in every mode, with no customer default")
	}
	for _, hint := range []string{"Paused", "confirm", "subscription", "cannot enable routing"} {
		if !strings.Contains(email.Description, hint) {
			t.Errorf("email setup lacks %s guidance", hint)
		}
	}
	run := f.Variables["setup_attempt"]
	if run.Type != "integer" || run.Required || run.Default != nil || run.Minimum != 0 || run.Maximum != 0 {
		t.Fatal("shared test number must have no implicit default; Terraform validates active bounds")
	}
	for _, hint := range []string{"same number", "Increase", "email", "package"} {
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
