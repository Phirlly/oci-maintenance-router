package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticReceiptConsumesPersistedAcceptanceWithoutEffects(t *testing.T) {
	f := coordinatorFixture(t)
	accepted, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || accepted.Record().Snapshot().State != DiagnosticAccepted {
		t.Fatal("diagnostic did not save acceptance", err)
	}
	want, _ := accepted.Record().Encode()
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		cfg := receiptConfig(t, accepted.Record(), func(c map[string]any) { c["mode"] = mode })
		for range 2 { // Reconstruct as after a restart; no mutable confirmation store.
			v := receiptVerifier(t, f, cfg)
			before := len(f.objects.calls)
			got, err := v.Verify(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || !bytes.Equal(body, want) || got.configHash != sha256.Sum256(cfg) || f.reads.Load() != 1 || f.sends.Load() != 1 {
				t.Fatal("saved acceptance and exact configuration were not bound", err)
			}
			receiptReadOnly(t, f, before)
			var order []string
			for _, c := range f.objects.calls[before:] {
				order = append(order, c.name)
			}
			if !reflect.DeepEqual(order, []string{workerLeaseName, "configuration/router.json", "configuration/catalog.json", diagnosticObjectName, workerLeaseName, "configuration/router.json", "configuration/catalog.json"}) {
				t.Fatal("missing or redundant guards", order)
			}
			clear(got.Record().Snapshot().PayloadWithoutRoutingKey)
			again, _ := got.Record().Encode()
			if !bytes.Equal(again, want) {
				t.Fatal("result exposes mutable record")
			}
		}
	}
	// Adding a receipt to the same TEST intent must not trigger another send.
	d, err := NewDiagnosticCoordinator(receiptConfig(t, accepted.Record()), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	d.current, d.send = f.coordinator.current, f.coordinator.send
	got, err := d.Run(leaseBudget(t), f.lease)
	if err != nil || got.Dispatched() || got.Record().Snapshot().State != DiagnosticAccepted || f.reads.Load() != 1 || f.sends.Load() != 1 {
		t.Fatal("receipt edit resumed a consumed test", err)
	}
}

func TestDiagnosticReceiptRejectsUnpersistedObserved202(t *testing.T) {
	f := coordinatorFixture(t)
	f.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == diagnosticObjectName {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	result, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err == nil || result.ObservedStatus() != 202 || result.Record().Snapshot().State != DiagnosticSending {
		t.Fatal("fixture did not lose outcome persistence", err)
	}
	f.objects.before = nil
	v := receiptVerifier(t, f, receiptConfig(t, result.Record()))
	before := len(f.objects.calls)
	got, err := v.Verify(leaseBudget(t), f.lease)
	receiptFailure(t, got, err, "not_accepted")
	receiptReadOnly(t, f, before)
	if f.reads.Load() != 1 || f.sends.Load() != 1 || f.saved(t).Snapshot().State != DiagnosticSending {
		t.Fatal("unpersisted acceptance was promoted or resent")
	}
}
