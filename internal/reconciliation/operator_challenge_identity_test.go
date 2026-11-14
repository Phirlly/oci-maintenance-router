package reconciliation

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOperatorChallengeIndependentMessageAndRecordGoldens(t *testing.T) {
	r := operatorChallengeFixture(t)
	s := r.Snapshot()
	want, err := os.ReadFile("testdata/operator-publishing.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Encode()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("independent publishing golden changed", err)
	}
	payload, err := os.ReadFile("testdata/operator-message.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, s.Payload) || bytes.Contains(got, []byte(operatorChannelFixture().Endpoint)) {
		t.Fatal("message changed or raw endpoint retained")
	}
	if !strings.HasPrefix(s.ConfirmationCode, "OMR-OP-") || len(s.ConfirmationCode) != 39 || !bytes.Contains(payload, []byte(s.ConfirmationCode)) {
		t.Fatal("wrong receipt correlation")
	}
}

func TestOperatorChallengeExactChannelAndIntentBinding(t *testing.T) {
	r := operatorChallengeFixture(t)
	i := operatorIntentFixture(t)
	if !r.Matches(i) || (OperatorChallenge{}).Matches(i) {
		t.Fatal("invalid original/zero match")
	}
	for _, change := range []func(*OperatorIntent){
		func(i *OperatorIntent) { i.EnvironmentID = strings.Repeat("f", 64) },
		func(i *OperatorIntent) { i.InstallationID = "other" }, func(i *OperatorIntent) { i.RequestID = "other" },
		func(i *OperatorIntent) { i.TopicID = "ocid1.onstopic.oc1.iad.other" },
		func(i *OperatorIntent) { i.SubscriptionID = "ocid1.onssubscription.oc1.iad.replaced" },
		func(i *OperatorIntent) { i.EndpointDigest = strings.Repeat("f", 64) },
	} {
		other := i
		change(&other)
		candidate, err := NewOperatorChallenge(other, operatorClaimFixture())
		if err != nil {
			t.Fatal(err)
		}
		if r.Matches(other) || candidate.Snapshot().IntentDigest == r.Snapshot().IntentDigest || candidate.Snapshot().ConfirmationCode == r.Snapshot().ConfirmationCode {
			t.Fatal("changed intent reused challenge")
		}
	}
	channel := operatorChannelFixture()
	channel.Endpoint = "Operator@example.invalid"
	other, err := NewOperatorIntent(i.EnvironmentID, i.InstallationID, i.RequestID, channel)
	if err != nil || other.EndpointDigest == i.EndpointDigest || r.Matches(other) {
		t.Fatal("endpoint was normalized", err)
	}
	c := operatorClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	c.Now = c.Now.Add(time.Second)
	c.ExpiresAt = c.ExpiresAt.Add(time.Second)
	next, err := NewOperatorChallenge(i, c)
	if err != nil || !next.Matches(i) || !bytes.Equal(next.Snapshot().Payload, r.Snapshot().Payload) {
		t.Fatal("claim/time changed channel challenge", err)
	}
}
