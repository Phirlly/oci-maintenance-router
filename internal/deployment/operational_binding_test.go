package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/reconciliation"
)

func TestOperationalCompositionRequiresManagedBindings(t *testing.T) {
	for _, kind := range []string{"storage", "unmanaged", "missing-principal", "invalid-region", "invalid-catalog-key", "invalid-config", "invalid-release"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _, principal := retainedTestFixture(t, false)
			if kind == "invalid-region" || kind == "invalid-config" {
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
			}
			p, failure := loadPinned(operationalContext(t), s, objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			var store Store = &managedStore{objects, principal}
			switch kind {
			case "storage":
				store = objects.pinned
			case "unmanaged":
				store = objects
			case "missing-principal":
				store = &managedStore{operationalObjects: objects}
			case "invalid-region":
				s.Region = "../invalid"
			case "invalid-catalog-key":
				delete(p.catalog.Regions, s.Region)
			case "invalid-config":
				p.configBytes = []byte("{}")
			case "invalid-release":
				s.ReleaseSHA256 = "not-a-release-digest"
			}
			ops, err := composeOperations(s, store, p, "deployment-check")
			if err == nil || ops.initialize != nil || ops.prepare != nil || ops.diagnostic != nil || ops.operator != nil || ops.diagnosticReceipt != nil || ops.pairedReceipts != nil || ops.selectLive != nil || ops.process != nil || principal.reads != 0 {
				t.Fatalf("invalid binding produced operational authority: %+v err=%v credential reads=%d", ops, err, principal.reads)
			}
		})
	}
}

func TestOperationalWorkerRequiresConditionalStorageBeforeComposing(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	h.connect = func(Settings) (Store, error) { return objects.pinned, nil }
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		t.Fatal("unusable storage reached composition")
		return operations{}, nil
	}
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "not-ready" || got.Code != "storage_configuration" || objects.pinned.creates != 0 {
		t.Fatalf("conditional storage missing: %+v", got)
	}
}

func TestPreviewCompositionUsesRealPreparationWithoutReadingCredentials(t *testing.T) {
	h, s, objects, _, principal := retainedTestFixture(t, false)
	s = configureOperational(t, h, s, objects, func(v map[string]any) {
		v["mode"] = "PREVIEW"
		v["preparation"] = map[string]any{"initialize": false}
	})
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "not-ready" || got.Code != "read_failed" || got.BaselinePrepared || principal.reads != 0 || len(objects.objects) != 1 {
		t.Fatalf("missing history did not stop real preparation: %+v", got)
	}
}

func TestNonLiveRecoveryFailureStopsPreparation(t *testing.T) {
	h, s, objects, environment, principal := retainedTestFixture(t, false)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PREVIEW" })
	configureRecovery(t, h, s, objects, environment, noticeledger.CloseRevision)
	objects.before = func(_ context.Context, method, name string) error {
		if method == "GET" && strings.HasPrefix(name, "notices/") {
			return errors.New("private storage failure")
		}
		return nil
	}
	h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
		ops, err := composeOperations(s, store, p, operation)
		ops.prepare = func(context.Context, *reconciliation.Lease) (reconciliation.Baseline, error) {
			t.Fatal("preparation ran after failed recovery")
			return reconciliation.Baseline{}, nil
		}
		return ops, err
	}
	got := invokeOperational(t, h, `{"operation":"reconcile"}`)
	if got.Status != "not-ready" || got.Code != "state_failed" || got.BaselinePrepared || got.ScanCompleted || principal.reads != 0 {
		t.Fatalf("recovery failure concealed: %+v", got)
	}
}

func TestLiveAndRecoveryCompositionRequireTrustedBindings(t *testing.T) {
	for _, kind := range []string{"live-release", "live-region", "recovery-location"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := liveOperationFixture(t)
			operation := "deployment-check"
			if kind == "recovery-location" {
				var environment string
				h, s, objects, environment = operationalFixture(t)
				s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
				s, _, _ = configureRecovery(t, h, s, objects, environment, noticeledger.CloseRevision)
				operation = "reconcile"
			}
			p, failure := loadPinned(operationalContext(t), s, objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			store, err := h.connect(s)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "live-region" {
				delete(p.catalog.Regions, s.Region)
			} else if kind == "recovery-location" {
				s.Bucket = "/invalid"
			} else {
				s.ReleaseSHA256 = "invalid"
			}
			ops, err := composeOperations(s, store, p, operation)
			if err == nil || ops.selectLive != nil || ops.process != nil {
				t.Fatal("unbound release, region or location produced operations")
			}
		})
	}
}
