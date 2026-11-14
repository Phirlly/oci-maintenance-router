package reconciliation

import (
	"bytes"
	"testing"
)

func TestLiveSchemaRetainsOneShotDiagnosticInTEST(t *testing.T) {
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
		diagnosticReconfigure(t, f, func(v map[string]any) { operatorTestConfig(v); v["schemaVersion"] = 6 })
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		body, _ := got.Record().Encode()
		if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || got.Dispatched() == existing || f.sends.Load() != 1 || f.reads.Load() != 1 {
			t.Fatal("schema6 changed one-shot diagnostic", err)
		}
		if existing && !bytes.Equal(prior, body) {
			t.Fatal("upgrade rewrote accepted diagnostic")
		}
		again, err := f.coordinator.Run(leaseBudget(t), f.lease)
		after, _ := again.Record().Encode()
		if err != nil || again.Dispatched() || !bytes.Equal(body, after) || f.sends.Load() != 1 {
			t.Fatal("same TEST intent resent or changed result", err)
		}
	}
}
func TestLiveSchemaRetainsOneShotOperatorPublicationInTEST(t *testing.T) {
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
		operatorReconfigure(t, f, func(v map[string]any) { v["schemaVersion"] = 6 })
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		body, _ := got.Record().Encode()
		if err != nil || got.Record().Snapshot().State != OperatorAccepted || got.Dispatched() == existing || f.sends.Load() != 1 || f.reads.Load() != 2 {
			t.Fatal("schema6 changed one-shot publication", err)
		}
		if existing && !bytes.Equal(prior, body) {
			t.Fatal("upgrade rewrote accepted publication")
		}
		again, err := f.coordinator.Run(leaseBudget(t), f.lease)
		after, _ := again.Record().Encode()
		if err != nil || again.Dispatched() || !bytes.Equal(body, after) || f.sends.Load() != 1 || f.reads.Load() != 2 {
			t.Fatal("same TEST intent republished", err)
		}
	}
}
