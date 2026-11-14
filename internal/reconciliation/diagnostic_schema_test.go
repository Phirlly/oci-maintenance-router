package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func operatorTestConfig(v map[string]any) {
	v["schemaVersion"] = 5
	v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "Team@example.invalid"}
}

func TestDiagnosticSchema5PreservesOneShotAndExistingReceipt(t *testing.T) {
	for _, existing := range []bool{false, true} {
		f := coordinatorFixture(t)
		if existing {
			if _, err := f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
				t.Fatal(err)
			}
		}
		diagnosticReconfigure(t, f, operatorTestConfig)
		result, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if err != nil || result.Record().Snapshot().State != DiagnosticAccepted || result.Dispatched() == existing || f.sends.Load() != 1 || f.reads.Load() != 1 {
			t.Fatal("schema5 changed one-shot diagnostic", err)
		}
		want, _ := result.Record().Encode()
		for _, endpoint := range []string{"Team@example.invalid", "Other@example.invalid"} {
			cfg := receiptConfig(t, result.Record(), func(v map[string]any) {
				operatorTestConfig(v)
				o := v["operatorTest"].(map[string]any)
				o["endpoint"] = endpoint
				o["receiptCode"] = "OMR-OP-0123456789abcdef0123456789abcdef"
			})
			verifier := receiptVerifier(t, f, cfg)
			before := len(f.objects.calls)
			got, err := verifier.Verify(leaseBudget(t), f.lease)
			actual, _ := got.Record().Encode()
			if err != nil || !bytes.Equal(actual, want) || got.configHash != sha256.Sum256(cfg) || f.sends.Load() != 1 || f.reads.Load() != 1 {
				t.Fatal("operator edit invalidated diagnostic receipt or resent", err)
			}
			receiptReadOnly(t, f, before)
		}
	}
}
