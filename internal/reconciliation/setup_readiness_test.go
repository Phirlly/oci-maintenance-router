package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func setupConfirmedRuntime(t *testing.T) *setupRuntimeFixture {
	t.Helper()
	f := setupRuntimeFixtureFor(t)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, "CONFIRM", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal(err)
	}
	return f
}

func setupReadinessFor(t *testing.T, f *setupRuntimeFixture, changes ...func(map[string]any)) *setupReadiness {
	t.Helper()
	cfg := setupRuntimeConfig(t, "LIVE", "NONE", 0, changes...)
	f.objects.seed("configuration/router.json", cfg)
	v, err := newSetupReadiness(cfg, []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	v.read = func(context.Context) (OperatorChannel, error) { return operatorChannelFixture(), nil }
	return v
}

func TestSetupReadinessVerifiesSavedProofWithoutTransientInputsOrWrites(t *testing.T) {
	f := setupConfirmedRuntime(t)
	v := setupReadinessFor(t, f)
	f.objects.calls = nil
	if err := v.verify(leaseBudget(t), f.lease, strings.Repeat("b", 64)); err != nil {
		t.Fatal("saved proof could not activate without codes", err)
	}
	if op, err := v.verifyOperator(leaseBudget(t), f.lease); err != nil || op.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("saved operator proof lost", err)
	}
	for _, call := range f.objects.calls {
		if call.method != "GET" {
			t.Fatal("readiness changed state", call.method)
		}
	}
	if f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("readiness sent a test")
	}
	v = setupReadinessFor(t, f, func(m map[string]any) { m["ownership"].(map[string]any)["key"] = "new-owner" })
	if err := v.verify(leaseBudget(t), f.lease, strings.Repeat("b", 64)); err != nil {
		t.Fatal("route edit invalidated independent proof", err)
	}
}

func TestSetupReadinessNeverFallsBackAfterNewFailedAttempt(t *testing.T) {
	f := setupConfirmedRuntime(t)
	f.configure(t, "SEND_TEST", 2)
	f.coordinator.diagnostic.current = func(context.Context, string) (diagnosticCredential, error) { return nil, errors.New("private") }
	if _, err := f.coordinator.Run(leaseBudget(t), f.lease); err == nil {
		t.Fatal("failed test accepted")
	}
	v := setupReadinessFor(t, f)
	var detail *SetupError
	if err := v.verify(leaseBudget(t), f.lease, strings.Repeat("b", 64)); !errors.As(err, &detail) || detail.Code != "confirmation_required" {
		t.Fatal("older confirmation reused", err)
	}
	if _, err := v.verifyOperator(leaseBudget(t), f.lease); err == nil {
		t.Fatal("operator bypassed incomplete current attempt")
	}
}

func TestSetupOperatorProofRemainsIndependentOfDiagnosticReadiness(t *testing.T) {
	for _, kind := range []string{"missing-diagnostic", "release-changed"} {
		t.Run(kind, func(t *testing.T) {
			f := setupConfirmedRuntime(t)
			v := setupReadinessFor(t, f)
			dID, _ := setupRequestIDs(f.lease.manager.environmentID, "installation-1", 1)
			name := "control/diagnostics/installation-1/" + dID + ".json"
			release := strings.Repeat("b", 64)
			if kind == "missing-diagnostic" {
				delete(f.objects.objects, name)
			} else {
				release = strings.Repeat("e", 64)
			}
			if err := v.verify(leaseBudget(t), f.lease, release); err == nil {
				t.Fatal("invalid diagnostic proof enabled processing")
			}
			f.objects.calls = nil
			if _, err := v.verifyOperator(leaseBudget(t), f.lease); err != nil {
				t.Fatal("diagnostic failure disabled operator reporting", err)
			}
			if preparationCalls(f.objects, "GET", name) != 0 {
				t.Fatal("operator reporting read unrelated diagnostic state")
			}
		})
	}
}
