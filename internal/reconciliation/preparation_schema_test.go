package reconciliation

import (
	"bytes"
	"testing"
)

func preparationSchemaThree(v map[string]any) { v["schemaVersion"] = 3 }
func TestPreparationAcceptsSchemaThreeAndPreservesExistingHistory(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		changes := []func(map[string]any){preparationSchemaThree}
		if upgrade {
			changes = nil
		}
		p, objects, lease := preparationFixture(t, nil, changes...)
		if err := p.Initialize(leaseBudget(t), lease); err != nil {
			t.Fatal(err)
		}
		before := prepareSuccessfully(t, p, lease)
		head := preparationHead(t, objects, lease)
		headBytes, _ := head.Encode()
		cfg := preparationConfig(t, preparationSchemaThree)
		p = preparationFor(t, objects, cfg, nil)
		if err := p.Initialize(leaseBudget(t), lease); err != nil {
			t.Fatal(err)
		}
		after := prepareSuccessfully(t, p, lease)
		newHead := preparationHead(t, objects, lease)
		got, _ := newHead.Encode()
		if before.ID() != after.ID() || !bytes.Equal(headBytes, got) {
			t.Fatal("schema3 reset cumulative prepared history")
		}
	}
}
func TestPreparationSchemaThreeRejectsTESTAndPAUSEDBeforeIO(t *testing.T) {
	for _, mode := range []string{"TEST", "PAUSED"} {
		p, objects, lease := preparationFixture(t, nil, func(v map[string]any) {
			v["schemaVersion"] = 3
			v["mode"] = mode
			v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.test", "nonProduction": true}
		})
		before := len(objects.calls)
		err := p.Initialize(leaseBudget(t), lease)
		preparationFailed(t, Baseline{}, err, "intent_required")
		got, err := p.Prepare(leaseBudget(t), lease)
		preparationFailed(t, got, err, "intent_required")
		if len(objects.calls) != before {
			t.Fatal("nonPREVIEW preparation performed I/O")
		}
	}
}
