package reconciliation

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

func TestOperatorReceiptConstructorOwnsInputsWithoutIO(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	r := f.seed(t, OperatorAccepted, false)
	cfg, cat := operatorReceiptConfig(t, r), []byte(preparationCatalog)
	before := len(f.objects.calls)
	v, err := NewOperatorReceiptVerifier(cfg, cat, candidateLocation(), f.objects, operatorClient(t))
	if err != nil || v == nil {
		t.Fatal("valid receipt constructor rejected", err)
	}
	hash := sha256.Sum256(cfg)
	clear(cfg)
	clear(cat)
	if v.input.current.configHash != hash || v.input.receiptCode != r.Snapshot().ConfirmationCode || v.input.intent != r.Snapshot().Intent || len(f.objects.calls) != before {
		t.Fatal("constructor lost owned input or performed I/O")
	}
	_, err = v.read(context.Background())
	if e, ok := err.(*ocinotifications.Error); !ok || e.Code != ocinotifications.DeadlineRequired {
		t.Fatal("reader not bound to actual adapter", err)
	}
}

func TestOperatorReceiptConstructorRejectsMissingOrInvalidIntent(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	r := f.seed(t, OperatorAccepted, false)
	for _, kind := range []string{"schema1", "schema2", "schema3", "schema4", "config", "catalog", "location", "objects", "typed-nil", "client", "no-intent", "no-code", "empty-code"} {
		t.Run(kind, func(t *testing.T) {
			cfg, cat := operatorReceiptConfig(t, r), []byte(preparationCatalog)
			location := candidateLocation()
			var objects OperatorChallengeObjects = f.objects
			client := operatorClient(t)
			want := "invalid_request"
			switch kind {
			case "schema1":
				cfg = []byte(candidateConfig)
			case "schema2":
				cfg = preparationConfig(t)
			case "schema3":
				cfg = diagnosticConfig(t)
			case "schema4":
				cfg = diagnosticConfig(t, func(c map[string]any) { c["schemaVersion"] = 4 })
			case "config":
				cfg = []byte("private config")
			case "catalog":
				cat = []byte("private catalog")
			case "location":
				location = configuration.LedgerLocation{}
			case "objects":
				objects = nil
			case "typed-nil":
				objects = (*preparationObjects)(nil)
			case "client":
				client = nil
			case "no-intent":
				cfg = operatorReceiptConfig(t, r, func(c map[string]any) { delete(c, "operatorTest") })
				want = "intent_required"
			case "no-code", "empty-code":
				cfg = operatorReceiptConfig(t, r, func(c map[string]any) {
					o := c["operatorTest"].(map[string]any)
					if kind == "no-code" {
						delete(o, "receiptCode")
					} else {
						o["receiptCode"] = ""
					}
				})
				want = "receipt_required"
			}
			before := len(f.objects.calls)
			v, err := NewOperatorReceiptVerifier(cfg, cat, location, objects, client)
			operatorRunFailure(t, err, want)
			if v != nil || len(f.objects.calls) != before || strings.Contains(err.Error(), "private") {
				t.Fatal("invalid constructor performed I/O, leaked input or returned verifier")
			}
		})
	}
}
