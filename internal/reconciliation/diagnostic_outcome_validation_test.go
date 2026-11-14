package reconciliation

import (
	"bytes"
	"testing"
	"time"
)

func TestDiagnosticInvalidOutcomesCannotChangeClaim(t *testing.T) {
	r := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	original, _ := r.Encode()
	for _, status := range []int{-1, 1, 99, 600} {
		got, err := r.Finish(diagnosticReference(c), status, c.Now)
		diagnosticRejected(t, got, err, "invalid_status")
	}
	ref := diagnosticReference(c)
	ref.OwnerToken = "wrong"
	got, err := r.Finish(ref, 202, c.Now)
	diagnosticRejected(t, got, err, "stale_claim")
	for _, at := range []time.Time{time.Time{}, c.Now.Add(-time.Millisecond), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		got, err := r.Finish(diagnosticReference(c), 202, at)
		diagnosticRejected(t, got, err, "invalid_time")
		got, err = r.Expire(at)
		diagnosticRejected(t, got, err, "invalid_time")
	}
	got, err = (DiagnosticRecord{}).Finish(diagnosticReference(c), 202, c.Now)
	diagnosticRejected(t, got, err, "invalid_record")
	got, err = (DiagnosticRecord{}).Expire(c.Now)
	diagnosticRejected(t, got, err, "invalid_record")
	if body, err := (DiagnosticRecord{}).Encode(); err == nil || body != nil {
		t.Fatal("zero record encoded")
	}
	actual, _ := r.Encode()
	if !bytes.Equal(original, actual) {
		t.Fatal("rejected operation changed immutable claim")
	}
}

func TestDiagnosticImmediateTransitionsAndTerminalNoops(t *testing.T) {
	r := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	accepted, err := r.Finish(diagnosticReference(c), 202, c.Now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if accepted.parent != r.data || accepted.data == r.data {
		t.Fatal("outcome not bound to immediate predecessor")
	}
	next, err := accepted.Expire(c.ExpiresAt)
	if err != nil || next.data != accepted.data {
		t.Fatal("terminal no-op fabricated a new transition")
	}
	got, err := accepted.Expire(c.Now)
	diagnosticRejected(t, got, err, "invalid_time")
	before, err := r.Expire(c.Now)
	if err != nil || before.data != r.data {
		t.Fatal("unexpired no-op changed claim")
	}
	expired, err := r.Expire(c.ExpiresAt)
	if err != nil || expired.parent != r.data {
		t.Fatal("expiry not bound to exact claim")
	}
	body, _ := accepted.Encode()
	i := diagnosticIntentFixture()
	loaded, err := DecodeDiagnostic(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil || loaded.parent != nil {
		t.Fatal("decoded record fabricated an immediate-parent link")
	}
	for _, record := range []DiagnosticRecord{accepted, expired} {
		s := record.Snapshot()
		if s.Intent != r.Snapshot().Intent || s.SecretVersion != 7 || s.ClaimToken != c.ClaimToken || !bytes.Equal(s.PayloadWithoutRoutingKey, r.Snapshot().PayloadWithoutRoutingKey) {
			t.Fatal("transition changed original request/pin/message")
		}
	}
}
