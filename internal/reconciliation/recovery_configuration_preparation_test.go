package reconciliation

import (
	"bytes"
	"testing"
)

func TestRecoveryConfigurationPreservesPreparationHistory(t *testing.T) {
	p, objects, lease := preparationFixture(t, nil)
	if err := p.Initialize(leaseBudget(t), lease); err != nil {
		t.Fatal(err)
	}
	before := prepareSuccessfully(t, p, lease)
	head, _ := preparationHead(t, objects, lease).Encode()
	for _, id := range []string{"recovery-1", "recovery-2"} {
		cfg := preparationConfig(t, func(v map[string]any) { recoveryConfiguration(v); v["recovery"].(map[string]any)["requestId"] = id })
		p = preparationFor(t, objects, cfg, nil)
		if err := p.Initialize(leaseBudget(t), lease); err != nil {
			t.Fatal(err)
		}
		after := prepareSuccessfully(t, p, lease)
		actual, _ := preparationHead(t, objects, lease).Encode()
		if before.ID() != after.ID() || !bytes.Equal(head, actual) {
			t.Fatal("recovery intent reset prepared history")
		}
	}
}

func TestRecoveryConfigurationCannotAuthorizePreparationOutsidePREVIEW(t *testing.T) {
	for _, mode := range []string{"TEST", "LIVE", "PAUSED"} {
		p, objects, lease := preparationFixture(t, nil)
		cfg := liveSchemaConfig(t, mode, recoveryConfiguration)
		next, err := NewBaselinePreparation(cfg, []byte(preparationCatalog), candidateLocation(), p.discovery.collector.announcements, objects)
		if err != nil {
			t.Fatal("valid configuration rejected before mode guard", err)
		}
		before := len(objects.calls)
		err = next.Initialize(leaseBudget(t), lease)
		preparationFailed(t, Baseline{}, err, "intent_required")
		got, err := next.Prepare(leaseBudget(t), lease)
		preparationFailed(t, got, err, "intent_required")
		if len(objects.calls) != before {
			t.Fatal("recovery bypassed preparation mode guard")
		}
	}
}
