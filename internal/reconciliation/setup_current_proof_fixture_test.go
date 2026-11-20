package reconciliation

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/pagerduty"
)

// Produce genuine verifier results using the existing accepted-record lifecycle.
// This fixture tests the state model, not schema8 deployment composition.
func setupCurrentProof(t *testing.T) (setupCurrent, SetupConfirmation) {
	t.Helper()
	_, objects, lease := preparationFixture(t, nil)
	dID, oID := setupRequestIDs(lease.manager.environmentID, "installation-1", 1)
	editIDs := func(v map[string]any) {
		v["diagnostic"].(map[string]any)["requestId"] = dID
		v["operatorTest"].(map[string]any)["requestId"] = oID
	}
	cfg := liveSchemaConfig(t, "TEST", editIDs)
	objects.seed("configuration/router.json", cfg)
	d, err := NewDiagnosticCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects, diagnosticVault(t))
	if err != nil {
		t.Fatal(err)
	}
	d.current = func(context.Context, string) (diagnosticCredential, error) {
		return diagnosticMaterial{7, func() []byte { return []byte("synthetic-key") }}, nil
	}
	d.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
		return pagerduty.Response{Status: 202}, nil
	}
	dr, err := d.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	o, err := NewOperatorCoordinator(cfg, []byte(preparationCatalog), candidateLocation(), objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	o.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{o.desired, func(context.Context, []byte) (ocinotifications.Publication, error) {
			return ocinotifications.Publication{Status: 200, MessageID: "synthetic-message", MayHaveSent: true}, nil
		}}, nil
	}
	or, err := o.Run(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	cfg = liveSchemaConfig(t, "TEST", editIDs, func(v map[string]any) {
		v["diagnostic"].(map[string]any)["receiptCode"] = dr.Record().Snapshot().ConfirmationCode
		v["operatorTest"].(map[string]any)["receiptCode"] = or.Record().Snapshot().ConfirmationCode
	})
	objects.seed("configuration/router.json", cfg)
	dv, err := NewDiagnosticReceiptVerifier(cfg, []byte(preparationCatalog), candidateLocation(), strings.Repeat("b", 64), objects)
	if err != nil {
		t.Fatal(err)
	}
	ov, err := NewOperatorReceiptVerifier(cfg, []byte(preparationCatalog), candidateLocation(), objects, operatorClient(t))
	if err != nil {
		t.Fatal(err)
	}
	ov.read = func(context.Context) (OperatorChannel, error) { return ov.input.desired, nil }
	dp, err := dv.Verify(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	op, err := ov.Verify(leaseBudget(t), lease)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := newSetupConfirmation(dp, op, lease.manager.now())
	if err != nil {
		t.Fatal(err)
	}
	head, err := newSetupCurrent(1, d.intent, o.intent)
	if err != nil {
		t.Fatal(err)
	}
	return head, proof
}
