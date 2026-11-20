package reconciliation

import (
	"bytes"
	"testing"
)

func TestSetupCurrentAttachesExactProofAndClearsItForNewAttempt(t *testing.T) {
	head, proof := setupCurrentProof(t)
	attached, err := head.attach(proof)
	body, _ := proof.Encode()
	if err != nil || attached.data == nil || attached.data.Confirmation == nil || attached.data.Confirmation.ID != proof.ID() || attached.data.Confirmation.SHA256 != diagnosticDigest(string(body)) || attached.parent != head.data {
		t.Fatal("confirmation was not bound to exact selected pair", err)
	}
	before, _ := attached.encode()
	repeated, err := attached.attach(proof)
	after, _ := repeated.encode()
	if err != nil || !bytes.Equal(before, after) || repeated.data != attached.data {
		t.Fatal("same confirmation changed selection", err)
	}
	d, o := setupCurrentIntents(t, 2)
	d.EnvironmentID, o.EnvironmentID = head.data.EnvironmentID, head.data.EnvironmentID
	d.RequestID, o.RequestID = setupRequestIDs(d.EnvironmentID, d.InstallationID, 2)
	next, err := attached.selectAttempt(2, d, o)
	if err != nil || next.data.Confirmation != nil || next.parent != attached.data || attached.data.Confirmation == nil {
		t.Fatal("new attempt retained old proof or mutated history", err)
	}
	if _, err := next.attach(proof); err == nil {
		t.Fatal("older confirmation attached to newer attempt")
	}
	if _, err := head.attach(SetupConfirmation{}); err == nil {
		t.Fatal("missing confirmation accepted")
	}
	changed := *proof.data
	changed.ConfigurationSHA256 = diagnosticDigest("another provenance")
	if _, err := attached.attach(SetupConfirmation{data: &changed}); err == nil {
		t.Fatal("same pair silently replaced immutable proof bytes")
	}
}
