package configuration

import (
	"bytes"
	"strings"
	"testing"
)

const receiptCode = "OMR-0123456789abcdef0123456789abcdef"

func receiptSettings(t *testing.T) map[string]any {
	t.Helper()
	v := diagnosticSettings(t)
	v["schemaVersion"] = 4
	v["diagnostic"].(map[string]any)["receiptCode"] = receiptCode
	return v
}

func TestReceiptControlsAreOptionalOwnedAndExact(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		for _, code := range []string{receiptCode, ""} {
			v := receiptSettings(t)
			v["mode"] = mode
			v["diagnostic"].(map[string]any)["receiptCode"] = code
			c := decoded(t, v)
			if c.SchemaVersion != 4 || c.Diagnostic.ReceiptCode != code || c.Mode != mode || c.Diagnostic.RequestID != "test-1" {
				t.Fatal("receipt input lost or changed")
			}
			v["diagnostic"].(map[string]any)["receiptCode"] = "changed"
			if c.Diagnostic.ReceiptCode != code {
				t.Fatal("receipt input aliases caller")
			}
			delete(v["diagnostic"].(map[string]any), "receiptCode")
			if decoded(t, v).Diagnostic.ReceiptCode != "" {
				t.Fatal("absent receipt gained attestation")
			}
			delete(v, "diagnostic")
			if mode == "TEST" {
				wantError(t, encoded(t, v), "diagnostic", "required_for_test")
			} else if decoded(t, v).Diagnostic != nil {
				t.Fatal("absent diagnostic gained intent")
			}
		}
	}
}

func TestReceiptControlsRejectMalformedOrLegacyAttestation(t *testing.T) {
	for _, code := range []string{"OMR-", strings.ToLower(receiptCode), strings.ToUpper(receiptCode), " " + receiptCode, receiptCode + "\n", receiptCode + "0", receiptCode[:len(receiptCode)-1], "OMR-" + strings.Repeat("g", 32), "é"} {
		v := receiptSettings(t)
		v["diagnostic"].(map[string]any)["receiptCode"] = code
		wantError(t, encoded(t, v), "diagnostic.receiptCode", "invalid")
	}
	for _, code := range []any{nil, true, 1, []any{}, map[string]any{}} {
		v := receiptSettings(t)
		v["diagnostic"].(map[string]any)["receiptCode"] = code
		wantError(t, encoded(t, v), "diagnostic.receiptCode", "type")
	}
	for _, version := range []int{1, 2, 3} {
		v := receiptSettings(t)
		v["schemaVersion"] = version
		field := "configuration"
		if version == 3 {
			field = "diagnostic"
		}
		wantError(t, encoded(t, v), field, "unknown_field")
	}
}

func TestReceiptControlsPreserveOriginalRoutingIdentity(t *testing.T) {
	original := decoded(t, settings(t))
	want, _ := original.canonicalRouting()
	location := LedgerLocation{Namespace: "example", Bucket: "ledger"}
	env, _ := original.EnvironmentID(location)
	for _, code := range []string{receiptCode, "", "OMR-" + strings.Repeat("a", 32)} {
		v := receiptSettings(t)
		v["diagnostic"].(map[string]any)["receiptCode"] = code
		c := decoded(t, v)
		got, _ := c.canonicalRouting()
		actual, _ := c.EnvironmentID(location)
		if !bytes.Equal(got, want) || actual != env || generation(t, c) != generation(t, original) {
			t.Fatal("receipt changed routing or environment identity")
		}
		v["routes"].([]any)[0].(map[string]any)["ownerValue"] = "other"
		if generation(t, decoded(t, v)) == generation(t, original) {
			t.Fatal("real routing edit lost generation change")
		}
	}
}
