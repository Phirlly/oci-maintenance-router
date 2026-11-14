package reconciliation

import (
	"strings"
	"testing"
)

func TestLiveSchemaPreparationRefusesLIVEBeforeIO(t *testing.T) {
	p, objects, lease := preparationFixture(t, nil)
	cfg := liveSchemaConfig(t, "LIVE")
	next, err := NewBaselinePreparation(cfg, []byte(preparationCatalog), candidateLocation(), p.discovery.collector.announcements, objects)
	if err != nil {
		t.Fatal("valid new schema rejected before mode check", err)
	}
	before := len(objects.calls)
	err = next.Initialize(leaseBudget(t), lease)
	preparationFailed(t, Baseline{}, err, "intent_required")
	got, err := next.Prepare(leaseBudget(t), lease)
	preparationFailed(t, got, err, "intent_required")
	if len(objects.calls) != before {
		t.Fatal("LIVE performed PREVIEW work")
	}
}

func TestLiveSchemaDiagnosticProducerRefusesLIVEBeforeIO(t *testing.T) {
	f := coordinatorFixture(t)
	before := len(f.objects.calls)
	got, err := NewDiagnosticCoordinator(liveSchemaConfig(t, "LIVE"), []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), f.objects, diagnosticVault(t))
	diagnosticRunFailure(t, err, "intent_required")
	if got != nil || len(f.objects.calls) != before || f.sends.Load() != 0 || f.reads.Load() != 0 {
		t.Fatal("LIVE granted diagnostic production")
	}
}
func TestLiveSchemaOperatorProducerRefusesLIVEBeforeIO(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	before := len(f.objects.calls)
	got, err := NewOperatorCoordinator(liveSchemaConfig(t, "LIVE"), []byte(preparationCatalog), candidateLocation(), f.objects, operatorClient(t))
	operatorRunFailure(t, err, "intent_required")
	if got != nil || len(f.objects.calls) != before || f.sends.Load() != 0 || f.reads.Load() != 0 {
		t.Fatal("LIVE granted operator test publication")
	}
}
