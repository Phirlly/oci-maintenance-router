package infra_test

import (
	"strings"
	"testing"
)

func TestSetupRenderingUsesSavedAuthorityWithoutTransientInputs(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "LIVE"} {
		t.Run(mode, func(t *testing.T) {
			s := setupSettings(mode, "NONE", true)
			// Hidden, stale values must not become active configuration authority.
			s["test_run"], s["operator_test_run"], s["setup_attempt"] = -1, -1, -1
			s["test_receipt_code"], s["operator_receipt_code"] = "stale", "stale"
			values, message, err := renderSetup(t, s)
			if err != nil {
				t.Fatalf("NONE rejected unused values: %s (%v)", message, err)
			}
			c := renderedConfig(t, values)
			if c.SchemaVersion != 8 || c.Mode != mode || c.Setup == nil || c.Setup.Action != "NONE" || c.Setup.Attempt != 0 || c.Diagnostic != nil || c.OperatorTest != nil {
				t.Fatalf("normal operation retained transient authority: %#v", c)
			}
			if c.OperatorChannel == nil || c.OperatorChannel.Endpoint != s["operator_email"] {
				t.Fatal("permanent operator channel lost")
			}
		})
	}
}

func TestSetupRenderingSeparatesSendFromPairedConfirmation(t *testing.T) {
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		t.Run(action, func(t *testing.T) {
			s := setupSettings("TEST", action, true)
			if action == "SEND_TEST" {
				s["test_receipt_code"], s["operator_receipt_code"] = "stale", "stale"
			}
			values, message, err := renderSetup(t, s)
			if err != nil {
				t.Fatalf("setup render: %s (%v)", message, err)
			}
			c := renderedConfig(t, values)
			if c.SchemaVersion != 8 || c.Setup == nil || c.Setup.Action != action || c.Setup.Attempt != 1 || c.OperatorChannel == nil || c.Diagnostic != nil || c.OperatorTest != nil {
				t.Fatalf("wrong paired setup contract: %#v", c)
			}
			if action == "CONFIRM" && (c.Setup.SecretID != "" || c.Setup.NonProduction || c.Setup.PagerDutyCode == "" || c.Setup.OperatorCode == "") {
				t.Fatal("confirmation requires retained send-only inputs")
			}
			if action == "SEND_TEST" && (!c.Setup.NonProduction || c.Setup.SecretID == "" || c.Setup.PagerDutyCode != "" || c.Setup.OperatorCode != "") {
				t.Fatal("SEND picked up stale confirmation")
			}
			if (values["confirmation"] == "true") != (action == "CONFIRM") {
				t.Fatal("explicit action does not govern confirmation")
			}
		})
	}
}

func TestDiagnosticOnlySetupHasSeparateIdentityAndCannotBecomeReadiness(t *testing.T) {
	var id string
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		values, message, err := renderSetup(t, setupSettings("TEST", action, false))
		if err != nil {
			t.Fatalf("diagnostic-only: %s (%v)", message, err)
		}
		c := renderedConfig(t, values)
		if c.SchemaVersion != 7 || c.Diagnostic == nil || c.OperatorTest != nil || c.Setup != nil || !strings.HasPrefix(c.Diagnostic.RequestID, "st1-") {
			t.Fatal("diagnostic-only path mixed with paired setup")
		}
		if id != "" && c.Diagnostic.RequestID != id {
			t.Fatal("confirmation changed the selected diagnostic")
		}
		id = c.Diagnostic.RequestID
	}
	if _, _, err := renderSetup(t, setupSettings("LIVE", "NONE", false)); err == nil {
		t.Fatal("diagnostic-only setup enabled routing without operator channel")
	}
}
