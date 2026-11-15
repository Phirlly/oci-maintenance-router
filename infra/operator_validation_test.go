package infra_test

import (
	"strings"
	"testing"
)

func TestOperatorRendererRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name, field, hint string
		value             any
	}{
		{"missing run", "operator_test_run", "Operator test run", nil},
		{"zero run", "operator_test_run", "Operator test run", 0},
		{"fraction", "operator_test_run", "Operator test run", 1.5},
		{"negative", "operator_test_run", "Operator test run", -1},
		{"overflow", "operator_test_run", "Operator test run", 2147483648},
		{"no at", "operator_email", "Operator email", "team.example.invalid"},
		{"two at", "operator_email", "Operator email", "ops@@example.invalid"},
		{"no local", "operator_email", "Operator email", "@example.invalid"},
		{"no domain", "operator_email", "Operator email", "ops@"},
		{"space", "operator_email", "Operator email", " ops@example.invalid"},
		{"line break", "operator_email", "Operator email", "ops\n@example.invalid"},
		{"unicode space", "operator_email", "Operator email", "ops\u00a0@example.invalid"},
		{"control", "operator_email", "Operator email", "ops\u0085@example.invalid"},
		{"scalar limit", "operator_email", "Operator email", strings.Repeat("a\u0338", 251) + "@example.io"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := operatorSettings()
			s[tc.field] = tc.value
			_, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
			if err == nil || !strings.Contains(message, tc.hint) {
				t.Fatalf("invalid %s accepted or wrong validation: %s (%v)", tc.name, message, err)
			}
		})
	}
	for _, subscription := range []string{"", "ocid1.onstopic.oc1.iad.wrong", "ocid1.onssubscription.oc1.phx.wrong"} {
		_, message, err := renderWithInstallation(t, operatorSettings(), "b1-"+strings.Repeat("a", 64), subscription)
		if err == nil || !strings.Contains(message, "Operator subscription") {
			t.Fatalf("invalid regional subscription: %s (%v)", message, err)
		}
	}
}

func TestOperatorEmailSetupDoesNotRequireTestIntent(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED"} {
		s := operatorSettings()
		s["mode"] = mode
		delete(s, "operator_test_run")
		values, message, err := render(t, s)
		if err != nil || renderedConfig(t, values).OperatorTest != nil {
			t.Fatalf("confirmation setup required test authority: %s (%v)", message, err)
		}
	}
}
