package infra_test

import (
	"strings"
	"testing"
)

func TestTestModeRejectsIncompleteOrInvalidIntent(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
		omit      bool
		message   string
	}{
		{"missing-run", "test_run", nil, true, "Test run number"},
		{"null-run", "test_run", nil, false, "Test run number"},
		{"zero-run", "test_run", 0, false, "Test run number"},
		{"negative-run", "test_run", -1, false, "Test run number"},
		{"fractional-run", "test_run", 1.5, false, "Test run number"},
		{"overflow-run", "test_run", 2147483648, false, "Test run number"},
		{"missing-secret", "test_secret_ocid", nil, true, "Test secret"},
		{"null-secret", "test_secret_ocid", nil, false, "Test secret"},
		{"oversized-secret", "test_secret_ocid", "ocid1.vaultsecret.oc1.iad." + strings.Repeat("a", 4096), false, "Test secret"},
		{"wrong-region", "test_secret_ocid", "ocid1.vaultsecret.oc1.phx.example", false, "Test secret"},
		{"missing-attestation", "test_nonproduction", nil, true, "nonproduction"},
		{"null-attestation", "test_nonproduction", nil, false, "nonproduction"},
		{"false-attestation", "test_nonproduction", false, false, "nonproduction"},
		{"live", "mode", "LIVE", false, "Mode"},
		{"unknown-mode", "mode", "test", false, "Mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := diagnosticSettings()
			if tc.omit {
				delete(s, tc.key)
			} else {
				s[tc.key] = tc.value
			}
			_, message, err := render(t, s)
			if err == nil || !strings.Contains(message, tc.message) {
				t.Fatalf("invalid test intent needs actionable error %q: %s (%v)", tc.message, message, err)
			}
		})
	}
}

func TestTestRunBoundsAndInactiveInputs(t *testing.T) {
	for _, run := range []int{1, 2147483647} {
		s := diagnosticSettings()
		s["test_run"] = run
		values, message, err := render(t, s)
		if err != nil {
			t.Fatalf("valid run: %s (%v)", message, err)
		}
		if renderedConfig(t, values).Diagnostic == nil {
			t.Fatal("valid explicit test missing")
		}
	}
	for _, mode := range []string{"PREVIEW", "PAUSED"} {
		s := diagnosticSettings()
		s["mode"], s["test_run"], s["test_secret_ocid"], s["test_nonproduction"] = mode, nil, "", false
		values, message, err := render(t, s)
		if err != nil || renderedConfig(t, values).Diagnostic != nil {
			t.Fatalf("inactive inputs should not block a safe mode: %s (%v)", message, err)
		}
	}
}
