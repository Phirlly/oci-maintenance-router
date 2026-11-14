package reconciliation

import (
	"context"
	"testing"
)

func operatorReceiptConfig(t *testing.T, r OperatorChallenge, changes ...func(map[string]any)) []byte {
	t.Helper()
	return diagnosticConfig(t, append([]func(map[string]any){operatorTestConfig, func(c map[string]any) {
		c["operatorTest"].(map[string]any)["receiptCode"] = r.Snapshot().ConfirmationCode
	}}, changes...)...)
}

func operatorReceiptVerifier(t *testing.T, f *operatorCoordinatorFixture, cfg []byte) *OperatorReceiptVerifier {
	t.Helper()
	v, err := NewOperatorReceiptVerifier(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal("valid operator receipt verifier rejected", err)
	}
	f.objects.seed("configuration/router.json", cfg)
	v.read = func(context.Context) (OperatorChannel, error) { f.reads.Add(1); return v.input.desired, nil }
	return v
}

func operatorReceiptFailure(t *testing.T, r OperatorReceipt, err error, code string) *OperatorRunError {
	t.Helper()
	e := operatorRunFailure(t, err, code)
	if r != (OperatorReceipt{}) || r.Record().Snapshot().State != "" {
		t.Fatal("failed verification returned receipt")
	}
	return e
}

func operatorReceiptReadOnly(t *testing.T, f *operatorCoordinatorFixture, start int) {
	t.Helper()
	for _, c := range f.objects.calls[start:] {
		if c.method != "GET" {
			t.Fatalf("verification performed %s", c.method)
		}
	}
}
