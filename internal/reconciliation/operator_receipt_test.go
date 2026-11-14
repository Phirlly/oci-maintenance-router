package reconciliation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorReceiptConsumesSavedAcceptanceWithCurrentChannelOnly(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	accepted, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || accepted.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("publication did not save acceptance", err)
	}
	want, _ := accepted.Record().Encode()
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		cfg := operatorReceiptConfig(t, accepted.Record(), func(c map[string]any) { c["mode"] = mode })
		for range 2 {
			v := operatorReceiptVerifier(t, f, cfg)
			before, reads := len(f.objects.calls), f.reads.Load()
			got, err := v.Verify(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || !bytes.Equal(body, want) || got.configHash != sha256.Sum256(cfg) || f.reads.Load() != reads+1 || f.sends.Load() != 1 {
				t.Fatal("saved acceptance/current configuration/channel not bound", err)
			}
			operatorReceiptReadOnly(t, f, before)
			var order []string
			for _, c := range f.objects.calls[before:] {
				order = append(order, c.name)
			}
			if !reflect.DeepEqual(order, []string{workerLeaseName, "configuration/router.json", "configuration/catalog.json", operatorRunObjectName, workerLeaseName, "configuration/router.json", "configuration/catalog.json"}) {
				t.Fatal("missing or redundant guards", order)
			}
			clear(got.Record().Snapshot().Payload)
			again, _ := got.Record().Encode()
			if !bytes.Equal(again, want) {
				t.Fatal("receipt exposes mutable state")
			}
		}
	}
}

func TestOperatorReceiptRejectsUnpersistedObservedPublication(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	f.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "REPLACE" && c.name == operatorRunObjectName {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	result, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err == nil || result.ObservedStatus() != 200 || result.ObservedMessageID() == "" || result.Record().Snapshot().State != OperatorPublishing {
		t.Fatal("fixture did not lose saved acceptance", err)
	}
	f.objects.before = nil
	v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, result.Record()))
	before, reads := len(f.objects.calls), f.reads.Load()
	got, err := v.Verify(leaseBudget(t), f.lease)
	operatorReceiptFailure(t, got, err, "not_accepted")
	operatorReceiptReadOnly(t, f, before)
	if f.reads.Load() != reads || f.sends.Load() != 1 || f.saved(t).Snapshot().State != OperatorPublishing {
		t.Fatal("unsaved acceptance promoted or resent")
	}
}
