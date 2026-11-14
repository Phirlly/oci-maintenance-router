package configuration

import "testing"

const operatorCode = "OMR-OP-0123456789abcdef0123456789abcdef"
const operatorSubscription = "ocid1.onssubscription.oc1.iad.operator"

func operatorSettings(t *testing.T) map[string]any {
	t.Helper()
	v := receiptSettings(t)
	v["schemaVersion"] = 5
	v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": operatorSubscription, "endpoint": "Team@example.invalid", "receiptCode": operatorCode}
	return v
}

func TestOperatorControlsPreserveExactOwnedIntent(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		v := operatorSettings(t)
		v["mode"] = mode
		c := decoded(t, v)
		want := OperatorTestRequest{"operator-1", operatorSubscription, "Team@example.invalid", operatorCode}
		if c.SchemaVersion != 5 || c.Mode != mode || c.OperatorTest == nil || *c.OperatorTest != want || c.Diagnostic.ReceiptCode != receiptCode {
			t.Fatal("operator or diagnostic controls lost")
		}
		second := decoded(t, v)
		c.OperatorTest.Endpoint = "changed"
		v["operatorTest"].(map[string]any)["requestId"] = "changed"
		if *second.OperatorTest != want {
			t.Fatal("operator intent aliases caller or another decode")
		}
		delete(v, "operatorTest")
		if decoded(t, v).OperatorTest != nil {
			t.Fatal("absent operator intent became requested")
		}
		delete(v, "diagnostic")
		if mode == "TEST" {
			wantError(t, encoded(t, v), "diagnostic", "required_for_test")
		} else if decoded(t, v).Diagnostic != nil {
			t.Fatal("optional diagnostic became requested")
		}
	}
}

func TestOperatorReceiptAbsentAndEmptyHaveNoAttestation(t *testing.T) {
	for _, omit := range []bool{false, true} {
		v := operatorSettings(t)
		o := v["operatorTest"].(map[string]any)
		o["receiptCode"] = ""
		if omit {
			delete(o, "receiptCode")
		}
		if decoded(t, v).OperatorTest.ReceiptCode != "" {
			t.Fatal("missing code gained attestation")
		}
	}
}

func TestOperatorIntentDoesNotReplaceRequiredPagerDutyTestIntent(t *testing.T) {
	v := operatorSettings(t)
	delete(v, "diagnostic")
	wantError(t, encoded(t, v), "diagnostic", "required_for_test")
}
