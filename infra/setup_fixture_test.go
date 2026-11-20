package infra_test

import (
	"strings"
	"testing"
)

func setupSettings(mode, action string, paired bool) map[string]any {
	s := settingsFixture()
	s["mode"], s["setup_action"] = mode, action
	if action != "NONE" {
		s["setup_attempt"] = 1
	}
	if paired {
		s["operator_email"] = "operator@example.invalid"
	}
	if action == "SEND_TEST" || action == "CONFIRM" && !paired {
		s["test_secret_ocid"], s["test_nonproduction"] = "ocid1.vaultsecret.oc1.iad.test", true
	}
	if action == "CONFIRM" {
		s["test_receipt_code"] = "OMR-" + strings.Repeat("a", 32)
		if paired {
			s["operator_receipt_code"] = "OMR-OP-" + strings.Repeat("b", 32)
		}
	}
	return s
}

func renderSetup(t *testing.T, s map[string]any) (map[string]string, string, error) {
	t.Helper()
	return renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
}
