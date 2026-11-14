package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestRecoveryConfigurationRetainsDiagnosticReceiptWithNewPins(t *testing.T) {
	f := coordinatorFixture(t)
	r := f.seed(t, DiagnosticAccepted, false)
	want, _ := r.Encode()
	old := receiptVerifier(t, f, receiptConfig(t, r))
	cfg := liveSchemaConfig(t, "LIVE", recoveryConfiguration, func(v map[string]any) {
		v["diagnostic"].(map[string]any)["receiptCode"] = r.Snapshot().ConfirmationCode
	})
	current := receiptVerifier(t, f, cfg)
	before := len(f.objects.calls)
	stale, err := old.Verify(leaseBudget(t), f.lease)
	receiptFailure(t, stale, err, "configuration_changed")
	got, err := current.Verify(leaseBudget(t), f.lease)
	body, _ := got.Record().Encode()
	if err != nil || !bytes.Equal(want, body) || got.configHash != sha256.Sum256(cfg) || f.sends.Load() != 0 || f.reads.Load() != 0 {
		t.Fatal("recovery changed diagnostic evidence", err)
	}
	receiptReadOnly(t, f, before)
}

func TestRecoveryConfigurationRetainsOperatorReceiptWithCurrentChannel(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	r := f.seed(t, OperatorAccepted, false)
	want, _ := r.Encode()
	old := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, r))
	cfg := liveSchemaConfig(t, "LIVE", recoveryConfiguration, func(v map[string]any) {
		v["operatorTest"].(map[string]any)["receiptCode"] = r.Snapshot().ConfirmationCode
	})
	current := operatorReceiptVerifier(t, f, cfg)
	before := len(f.objects.calls)
	stale, err := old.Verify(leaseBudget(t), f.lease)
	operatorReceiptFailure(t, stale, err, "configuration_changed")
	got, err := current.Verify(leaseBudget(t), f.lease)
	body, _ := got.Record().Encode()
	if err != nil || !bytes.Equal(want, body) || got.configHash != sha256.Sum256(cfg) || f.sends.Load() != 0 || f.reads.Load() != 1 {
		t.Fatal("recovery changed operator evidence", err)
	}
	operatorReceiptReadOnly(t, f, before)
}
