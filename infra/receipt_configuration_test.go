package infra_test

import (
	"strings"
	"testing"
)

func receiptSettings() map[string]any {
	s := operatorSettings()
	s["test_receipt_code"] = "OMR-" + strings.Repeat("a", 32)
	s["operator_receipt_code"] = "OMR-OP-" + strings.Repeat("b", 32)
	return s
}

func TestReceiptRendererPreservesCodesWithoutChangingTestIdentity(t *testing.T) {
	for _, operator := range []bool{false, true} {
		s := receiptSettings()
		if !operator {
			delete(s, "operator_email")
			delete(s, "operator_test_run")
			delete(s, "operator_receipt_code")
		}
		values, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
		if err != nil {
			t.Fatalf("receipt render: %s (%v)", message, err)
		}
		c := renderedConfig(t, values)
		if values["confirmation"] != "true" {
			t.Fatal("valid receipt input did not select confirmation permissions")
		}
		if c.Diagnostic.ReceiptCode != s["test_receipt_code"] || operator && (c.OperatorTest == nil || c.OperatorTest.ReceiptCode != s["operator_receipt_code"]) {
			t.Fatal("explicit confirmation was discarded")
		}
		delete(s, "test_receipt_code")
		delete(s, "operator_receipt_code")
		before, message, err := renderWithInstallation(t, s, c.InstallationID, "ocid1.onssubscription.oc1.iad.operator")
		if err != nil {
			t.Fatalf("send render: %s (%v)", message, err)
		}
		old := renderedConfig(t, before)
		if before["confirmation"] != "false" {
			t.Fatal("omitted receipts unexpectedly select confirmation")
		}
		if c.Diagnostic.RequestID != old.Diagnostic.RequestID || c.InstallationID != old.InstallationID || operator && c.OperatorTest.RequestID != old.OperatorTest.RequestID {
			t.Fatal("receipt entry created a new test identity")
		}
	}
}

func TestReceiptInputsRejectIncompleteOrMalformedTestConfirmation(t *testing.T) {
	for _, kind := range []string{"missing-diagnostic", "missing-operator", "orphan", "wrong-prefix", "uppercase", "space", "newline", "numeric", "wrong-type"} {
		t.Run(kind, func(t *testing.T) {
			s := receiptSettings()
			switch kind {
			case "missing-diagnostic":
				delete(s, "test_receipt_code")
			case "missing-operator":
				delete(s, "operator_receipt_code")
			case "orphan":
				delete(s, "operator_email")
				delete(s, "test_receipt_code")
			case "wrong-prefix":
				s["operator_receipt_code"] = s["test_receipt_code"]
			case "uppercase":
				s["test_receipt_code"] = "OMR-" + strings.Repeat("A", 32)
			case "space":
				s["test_receipt_code"] = " " + s["test_receipt_code"].(string)
			case "newline":
				s["operator_receipt_code"] = s["operator_receipt_code"].(string) + "\n"
			case "numeric":
				s["test_receipt_code"] = 123
			case "wrong-type":
				s["test_receipt_code"] = []string{"code"}
			}
			_, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
			if err == nil || kind != "wrong-type" && !strings.Contains(message, "confirmation") {
				t.Fatalf("invalid confirmation silently allowed: %s (%v)", message, err)
			}
		})
	}
}

func TestEmptyReceiptControlsAndInactiveModesPreserveConfiguration(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		s := operatorSettings()
		s["mode"] = mode
		before, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
		if err != nil {
			t.Fatalf("render: %s (%v)", message, err)
		}
		inputs := []any{nil, ""}
		if mode != "TEST" {
			inputs = append(inputs, "invalid inactive code")
		}
		for _, input := range inputs {
			s["test_receipt_code"], s["operator_receipt_code"] = input, input
			after, message, err := renderWithInstallation(t, s, "b1-"+strings.Repeat("a", 64), "ocid1.onssubscription.oc1.iad.operator")
			if err != nil || before["configuration"] != after["configuration"] || after["confirmation"] != "false" {
				t.Fatalf("empty/inactive receipt changed %s configuration: %s (%v)", mode, message, err)
			}
		}
	}
}
