package reconciliation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func operatorChannelFixture() OperatorChannel {
	return OperatorChannel{TopicID: "ocid1.onstopic.oc1.iad.operator", SubscriptionID: "ocid1.onssubscription.oc1.iad.operator", Protocol: "EMAIL", Endpoint: "operator@example.invalid"}
}
func operatorIntentFixture(t *testing.T) OperatorIntent {
	t.Helper()
	i, err := NewOperatorIntent(strings.Repeat("a", 64), "installation-1", "operator-test-1", operatorChannelFixture())
	if err != nil {
		t.Fatal("valid operator intent rejected", err)
	}
	return i
}
func operatorClaimFixture() OperatorClaim {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	return OperatorClaim{OwnerToken: strings.Repeat("d", 64), ClaimToken: strings.Repeat("e", 64), Now: now, ExpiresAt: now.Add(600 * time.Second)}
}
func operatorReference(c OperatorClaim) OperatorClaimRef {
	return OperatorClaimRef{c.OwnerToken, c.ClaimToken}
}
func operatorChallengeFixture(t *testing.T) OperatorChallenge {
	t.Helper()
	r, err := NewOperatorChallenge(operatorIntentFixture(t), operatorClaimFixture())
	if err != nil || r.Snapshot().State != OperatorPublishing {
		t.Fatal("valid operator claim rejected", err)
	}
	return r
}
func operatorRejected(t *testing.T, r OperatorChallenge, err error, code string) {
	t.Helper()
	var detail *OperatorChallengeError
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v; want %s", err, code)
	}
	if r.Snapshot().State != "" || r.Snapshot().Payload != nil {
		t.Fatal("rejected challenge returned partial state")
	}
}
