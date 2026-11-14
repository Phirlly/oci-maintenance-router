package reconciliation

import (
	"bytes"
	"testing"
)

func TestOperatorCoordinatorExplicitNewRequestIsIndependent(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	first, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := first.Record().Encode()
	operatorReconfigure(t, f, func(c map[string]any) { c["operatorTest"].(map[string]any)["requestId"] = "operator-2" })
	second, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || !second.Dispatched() || second.Record().Snapshot().State != OperatorAccepted || f.sends.Load() != 2 || first.Record().Snapshot().IntentDigest == second.Record().Snapshot().IntentDigest {
		t.Fatal("new explicit request not independent", err)
	}
	f.objects.mu.Lock()
	old := bytes.Clone(f.objects.objects[operatorRunObjectName].Body)
	f.objects.mu.Unlock()
	if !bytes.Equal(before, old) {
		t.Fatal("new request overwrote consumed history")
	}
}

func TestOperatorCoordinatorReceiptAndRoutingEditsDoNotResend(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	first, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := first.Record().Encode()
	operatorReconfigure(t, f, func(c map[string]any) {
		c["operatorTest"].(map[string]any)["receiptCode"] = first.Record().Snapshot().ConfirmationCode
		c["ownership"].(map[string]any)["key"] = "team"
	})
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	after, _ := got.Record().Encode()
	if err != nil || got.Dispatched() || !bytes.Equal(before, after) || f.sends.Load() != 1 || f.reads.Load() != 2 {
		t.Fatal("receipt/routing edits reset one-shot identity", err)
	}
}
