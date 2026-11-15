package reconciliation

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOperatorChallengeRejectsInvalidPublicationOutcomes(t *testing.T) {
	r := operatorChallengeFixture(t)
	c := operatorClaimFixture()
	before, _ := r.Encode()
	for _, status := range []int{-1, 1, 99, 600} {
		got, err := r.Finish(operatorReference(c), status, "", c.Now)
		operatorRejected(t, got, err, "invalid_status")
	}
	for _, id := range []string{"private bad", "private\n", string([]byte{255}), strings.Repeat("x", 257), "\u00a0", "\u0080"} {
		got, err := r.Finish(operatorReference(c), 200, id, c.Now)
		operatorRejected(t, got, err, "invalid_message_id")
		if strings.Contains(err.Error(), id) {
			t.Fatal("message identifier leaked")
		}
	}
	for _, status := range []int{0, 201, 400, 503} {
		got, err := r.Finish(operatorReference(c), status, "message", c.Now)
		operatorRejected(t, got, err, "invalid_message_id")
	}
	for _, ref := range []OperatorClaimRef{{"wrong", c.ClaimToken}, {c.OwnerToken, "wrong"}} {
		got, err := r.Finish(ref, 200, "message", c.Now)
		operatorRejected(t, got, err, "stale_claim")
	}
	for _, at := range []time.Time{time.Time{}, c.Now.Add(-time.Millisecond), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		got, err := r.Finish(operatorReference(c), 200, "message", at)
		operatorRejected(t, got, err, "invalid_time")
		got, err = r.Expire(at)
		operatorRejected(t, got, err, "invalid_time")
	}
	got, err := (OperatorChallenge{}).Finish(operatorReference(c), 200, "message", c.Now)
	operatorRejected(t, got, err, "invalid_record")
	got, err = (OperatorChallenge{}).Expire(c.Now)
	operatorRejected(t, got, err, "invalid_record")
	if body, err := (OperatorChallenge{}).Encode(); err == nil || body != nil {
		t.Fatal("zero challenge encoded")
	}
	after, _ := r.Encode()
	if !bytes.Equal(before, after) {
		t.Fatal("rejected publication changed claim")
	}
}

func TestOperatorChallengeTransitionsRetainExactPredecessor(t *testing.T) {
	r := operatorChallengeFixture(t)
	c := operatorClaimFixture()
	accepted, err := r.Finish(operatorReference(c), 200, "message", c.Now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if accepted.parent != r.data || accepted.data == r.data {
		t.Fatal("outcome lost immediate predecessor")
	}
	noOp, err := accepted.Expire(c.ExpiresAt)
	if err != nil || noOp.data != accepted.data {
		t.Fatal("terminal expiry fabricated transition", err)
	}
	bad, err := accepted.Expire(c.Now)
	operatorRejected(t, bad, err, "invalid_time")
	noOp, err = r.Expire(c.Now)
	if err != nil || noOp.data != r.data {
		t.Fatal("unexpired claim changed", err)
	}
	expired, err := r.Expire(c.ExpiresAt)
	if err != nil || expired.parent != r.data {
		t.Fatal("expiry lost predecessor", err)
	}
	body, _ := accepted.Encode()
	i := r.Snapshot().Intent
	loaded, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil || loaded.parent != nil {
		t.Fatal("restart invented parent", err)
	}
}
