package reconciliation

import (
	"strings"
	"testing"
	"time"
)

func TestOperatorChallengeClockAndTokenBoundaries(t *testing.T) {
	for _, change := range []func(*OperatorClaim){
		func(c *OperatorClaim) { c.OwnerToken = "private" }, func(c *OperatorClaim) { c.OwnerToken = strings.Repeat("D", 64) },
		func(c *OperatorClaim) { c.ClaimToken = strings.Repeat("e", 63) }, func(c *OperatorClaim) { c.Now = time.Time{} },
		func(c *OperatorClaim) { c.ExpiresAt = time.Time{} }, func(c *OperatorClaim) { c.Now = time.Date(0, 12, 31, 0, 0, 0, 0, time.UTC) },
		func(c *OperatorClaim) { c.ExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		func(c *OperatorClaim) { c.ExpiresAt = c.Now }, func(c *OperatorClaim) { c.ExpiresAt = c.Now.Add(-time.Millisecond) },
		func(c *OperatorClaim) { c.ExpiresAt = c.Now.Add(time.Microsecond) }, func(c *OperatorClaim) { c.ExpiresAt = c.Now.Add(600*time.Second + time.Millisecond) },
	} {
		c := operatorClaimFixture()
		change(&c)
		r, err := NewOperatorChallenge(operatorIntentFixture(t), c)
		operatorRejected(t, r, err, "invalid_claim")
	}
	c := operatorClaimFixture()
	c.Now = c.Now.In(time.FixedZone("offset", 2*60*60)).Add(1234567 * time.Nanosecond)
	c.ExpiresAt = c.Now.Add(time.Millisecond)
	r, err := NewOperatorChallenge(operatorIntentFixture(t), c)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Snapshot()
	if s.StartedAt.Location() != time.UTC || s.StartedAt.Nanosecond() != 1000000 || s.ExpiresAt.Sub(s.StartedAt) != time.Millisecond {
		t.Fatal("clocks were not canonical UTC milliseconds")
	}
}

func TestOperatorChallengeSeparateOperationsCannotFinishEachOther(t *testing.T) {
	a := operatorChallengeFixture(t)
	c := operatorClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	b, err := NewOperatorChallenge(operatorIntentFixture(t), c)
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().OwnerToken != b.Snapshot().OwnerToken || a.Snapshot().ClaimToken == b.Snapshot().ClaimToken {
		t.Fatal("operation identity collapsed into lease")
	}
	wrong, err := a.Finish(operatorReference(c), 200, "message", c.Now)
	operatorRejected(t, wrong, err, "stale_claim")
	wrong, err = b.Finish(operatorReference(operatorClaimFixture()), 200, "message", c.Now)
	operatorRejected(t, wrong, err, "stale_claim")
}
