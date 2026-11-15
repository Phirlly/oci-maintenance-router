package infra_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestLiveRendererRetainsVerifiedIntentAcrossModeChanges(t *testing.T) {
	s := receiptSettings()
	installation, subscription := "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator"
	before, message, err := renderWithInstallation(t, s, installation, subscription)
	if err != nil {
		t.Fatalf("TEST: %s (%v)", message, err)
	}
	proof := renderedConfig(t, before)
	for _, mode := range []string{"LIVE", "PAUSED", "PREVIEW", "LIVE", "TEST"} {
		s["mode"] = mode
		values, message, err := renderWithInstallation(t, s, installation, subscription)
		if err != nil {
			t.Fatalf("%s: %s (%v)", mode, message, err)
		}
		got := renderedConfig(t, values)
		if got.Mode != mode || got.Recovery != nil || got.ActiveBaselineID != "" {
			t.Fatal("unexpected mode or recovery/baseline authority")
		}
		if mode == "LIVE" || mode == "TEST" {
			if !reflect.DeepEqual(got.Diagnostic, proof.Diagnostic) || !reflect.DeepEqual(got.OperatorTest, proof.OperatorTest) {
				t.Fatal("mode transition changed saved proof identity")
			}
		} else if got.Diagnostic != nil || got.OperatorTest != nil {
			t.Fatal("inactive mode acquired proof/test authority")
		}
		if values["confirmation"] != "false" && mode != "TEST" {
			t.Fatal("LIVE must not select TEST result expectations")
		}
	}
}

func TestLiveRendererRejectsMissingOrInvalidPrerequisites(t *testing.T) {
	for _, tc := range []struct {
		field   string
		value   any
		message string
	}{
		{"test_receipt_code", "", "LIVE"},
		{"operator_receipt_code", nil, "LIVE"},
		{"test_receipt_code", "OMR-" + strings.Repeat("A", 32), "confirmation"},
		{"operator_receipt_code", "OMR-" + strings.Repeat("a", 32), "confirmation"},
		{"operator_email", "", "LIVE"},
		{"test_run", nil, "Test run number"},
		{"test_run", 0, "Test run number"},
		{"test_run", 2147483648, "Test run number"},
		{"operator_test_run", nil, "Operator test run"},
		{"operator_test_run", 1.5, "Operator test run"},
		{"test_secret_ocid", "ocid1.vaultsecret.oc1.phx.example", "Test secret"},
		{"test_nonproduction", false, "nonproduction"},
		{"closure_confirm", true, "PAUSED"},
	} {
		t.Run(tc.field+"/"+tc.message, func(t *testing.T) {
			s := receiptSettings()
			s["mode"], s[tc.field] = "LIVE", tc.value
			_, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
			if err == nil || !strings.Contains(message, tc.message) {
				t.Fatalf("want rejection %q: %s (%v)", tc.message, message, err)
			}
		})
	}
	s := receiptSettings()
	s["mode"] = "LIVE"
	_, message, err := render(t, s)
	if err == nil || !strings.Contains(message, "Operator subscription") {
		t.Fatalf("missing deployment-owned subscription: %s (%v)", message, err)
	}
}
