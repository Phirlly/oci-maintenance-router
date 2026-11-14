package reconciliation

import (
	"strings"
	"testing"
)

func receiptConfig(t *testing.T, r DiagnosticRecord, changes ...func(map[string]any)) []byte {
	t.Helper()
	return diagnosticConfig(t, append([]func(map[string]any){func(v map[string]any) {
		v["schemaVersion"] = 4
		v["diagnostic"].(map[string]any)["receiptCode"] = r.Snapshot().ConfirmationCode
	}}, changes...)...)
}

func receiptVerifier(t *testing.T, f *diagnosticCoordinatorFixture, cfg []byte) *DiagnosticReceiptVerifier {
	t.Helper()
	v, err := NewDiagnosticReceiptVerifier(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects)
	if err != nil {
		t.Fatal("valid receipt verifier rejected", err)
	}
	f.objects.seed("configuration/router.json", cfg)
	return v
}

func receiptFailure(t *testing.T, r DiagnosticReceipt, err error, code string) *DiagnosticRunError {
	t.Helper()
	e := diagnosticRunFailure(t, err, code)
	if r != (DiagnosticReceipt{}) || r.Record().Snapshot().State != "" {
		t.Fatal("failed verification returned proof")
	}
	return e
}

func receiptReadOnly(t *testing.T, f *diagnosticCoordinatorFixture, start int) {
	t.Helper()
	for _, c := range f.objects.calls[start:] {
		if c.method != "GET" {
			t.Fatalf("verification performed %s", c.method)
		}
	}
}
