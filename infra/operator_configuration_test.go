package infra_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func operatorSettings() map[string]any {
	s := diagnosticSettings()
	s["operator_email"] = "Ops+Router@Example.invalid"
	s["operator_test_run"] = 3
	return s
}

func TestOperatorTestRequiresDeploymentOwnedSubscription(t *testing.T) {
	// This assertion also runs against the old module: silently ignoring an
	// explicit operator test is not successful setup.
	_, message, err := render(t, operatorSettings())
	if err == nil || !strings.Contains(message, "Operator subscription") {
		t.Fatalf("missing deployment subscription must fail: %s (%v)", message, err)
	}
}

func TestOperatorControlsOmittedPreserveConfigurationBytes(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		s := diagnosticSettings()
		s["mode"] = mode
		before, message, err := render(t, s)
		if err != nil {
			t.Fatalf("legacy render: %s (%v)", message, err)
		}
		for _, explicit := range []any{nil, ""} {
			s["operator_email"] = explicit
			s["operator_test_run"] = nil
			after, message, err := render(t, s)
			if err != nil || after["configuration"] != before["configuration"] {
				t.Fatalf("omitted operator controls changed %s configuration: %s (%v)", mode, message, err)
			}
		}
	}
}

func TestOperatorRendererUsesExactContactAndStableExplicitRequest(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		s := operatorSettings()
		s["mode"] = mode
		values, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
		if err != nil {
			t.Fatalf("render: %s (%v)", message, err)
		}
		c := renderedConfig(t, values)
		if mode != "TEST" {
			if c.OperatorTest != nil {
				t.Fatal("inactive mode authorized operator publication")
			}
			continue
		}
		sum := sha256.Sum256([]byte(`["operator-request-v1","b1-` + strings.Repeat("a", 64) + `",3]`))
		if c.OperatorTest == nil || c.OperatorTest.RequestID != "o1-"+hex.EncodeToString(sum[:]) || c.OperatorTest.Endpoint != s["operator_email"] || c.OperatorTest.SubscriptionID != "ocid1.onssubscription.oc1.iad.operator" || c.OperatorTest.ReceiptCode != "" || c.Diagnostic == nil {
			t.Fatal("operator configuration lost exact intent or granted receipt authority")
		}
	}
}
