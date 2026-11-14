package reconciliation

import (
	"bytes"
	"strings"
	"testing"
)

func TestRecoveryConfigurationPreservesOneShotDiagnostic(t *testing.T) {
	for _, existing := range []bool{false, true} {
		f := coordinatorFixture(t)
		var prior []byte
		if existing {
			r, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			prior, _ = r.Record().Encode()
		}
		for i, id := range []string{"recovery-1", "recovery-2"} {
			diagnosticReconfigure(t, f, func(v map[string]any) { recoveryConfiguration(v); v["recovery"].(map[string]any)["requestId"] = id })
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || got.Dispatched() != (!existing && i == 0) || f.sends.Load() != 1 || f.reads.Load() != 1 {
				t.Fatal("recovery edit replayed or lost diagnostic", err)
			}
			if prior != nil && !bytes.Equal(prior, body) {
				t.Fatal("recovery edit rewrote diagnostic outcome")
			}
			prior = body
		}
	}
}

func TestRecoveryConfigurationPreservesOneShotOperatorPublication(t *testing.T) {
	for _, existing := range []bool{false, true} {
		f := operatorCoordinatorFixtureFor(t)
		var prior []byte
		if existing {
			r, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil {
				t.Fatal(err)
			}
			prior, _ = r.Record().Encode()
		}
		for i, id := range []string{"recovery-1", "recovery-2"} {
			operatorReconfigure(t, f, func(v map[string]any) { recoveryConfiguration(v); v["recovery"].(map[string]any)["requestId"] = id })
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			body, _ := got.Record().Encode()
			if err != nil || got.Record().Snapshot().State != OperatorAccepted || got.Dispatched() != (!existing && i == 0) || f.sends.Load() != 1 || f.reads.Load() != 2 {
				t.Fatal("recovery edit republished or lost operator test", err)
			}
			if prior != nil && !bytes.Equal(prior, body) {
				t.Fatal("recovery edit rewrote operator outcome")
			}
			prior = body
		}
	}
}

func TestRecoveryConfigurationCannotAuthorizeTestProducersOutsideTEST(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "LIVE"} {
		cfg := liveSchemaConfig(t, mode, recoveryConfiguration)
		d := coordinatorFixture(t)
		before := len(d.objects.calls)
		got, err := NewDiagnosticCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), d.objects, diagnosticVault(t))
		diagnosticRunFailure(t, err, "intent_required")
		if got != nil || len(d.objects.calls) != before || d.sends.Load() != 0 || d.reads.Load() != 0 {
			t.Fatal("nonTEST diagnostic I/O")
		}
		o := operatorCoordinatorFixtureFor(t)
		before = len(o.objects.calls)
		pub, err := NewOperatorCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), o.objects, operatorClient(t))
		operatorRunFailure(t, err, "intent_required")
		if pub != nil || len(o.objects.calls) != before || o.sends.Load() != 0 || o.reads.Load() != 0 {
			t.Fatal("nonTEST publication I/O")
		}
	}
}
