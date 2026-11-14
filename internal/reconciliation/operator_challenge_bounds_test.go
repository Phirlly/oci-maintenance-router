package reconciliation

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOperatorChallengeMaximumFieldsFitAllStates(t *testing.T) {
	i := operatorIntentFixture(t)
	i.InstallationID = strings.Repeat("I", 128)
	i.RequestID = strings.Repeat("R", 128)
	prefix := "ocid1.onstopic.oc1.iad."
	i.TopicID = prefix + strings.Repeat("a", 4096-len(prefix))
	prefix = "ocid1.onssubscription.oc1.iad."
	i.SubscriptionID = prefix + strings.Repeat("b", 4096-len(prefix))
	c := operatorClaimFixture()
	c.Now = time.Date(9999, 12, 31, 23, 49, 59, 999000000, time.UTC)
	c.ExpiresAt = c.Now.Add(600 * time.Second)
	r, err := NewOperatorChallenge(i, c)
	if err != nil {
		t.Fatal(err)
	}
	records := []OperatorChallenge{r}
	for _, tc := range []struct {
		status int
		id     string
	}{{0, ""}, {200, ""}, {200, strings.Repeat("<", 256)}, {200, strings.Repeat("🙂", 64)}, {202, ""}, {429, ""}, {599, ""}} {
		next, err := r.Finish(operatorReference(c), tc.status, tc.id, c.ExpiresAt.Add(-time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, next)
	}
	expired, err := r.Expire(c.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	records = append(records, expired)
	for _, record := range records {
		body, err := record.Encode()
		if err != nil || len(body) >= operatorChallengeLimit {
			t.Fatal("maximum record exceeded bound", len(body), err)
		}
		if len(record.Snapshot().Payload) > 64<<10 {
			t.Fatal("fixed message exceeds ONS bound")
		}
		loaded, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err != nil {
			t.Fatal("maximum state failed restart", err)
		}
		encoded, _ := loaded.Encode()
		if !bytes.Equal(body, encoded) {
			t.Fatal("restart changed maximum state")
		}
	}
	for _, change := range []func(*OperatorIntent){func(i *OperatorIntent) { i.TopicID += "a" }, func(i *OperatorIntent) { i.SubscriptionID += "a" }, func(i *OperatorIntent) { i.InstallationID += "a" }, func(i *OperatorIntent) { i.RequestID += "a" }} {
		bad := i
		change(&bad)
		got, err := NewOperatorChallenge(bad, c)
		operatorRejected(t, got, err, "invalid_intent")
	}
	bad, err := r.Finish(operatorReference(c), 200, strings.Repeat("🙂", 64)+"a", c.Now)
	operatorRejected(t, bad, err, "invalid_message_id")
}

func TestOperatorChallengePreservesSupportedRegionalReferenceForms(t *testing.T) {
	for _, suffix := range []string{"us-ashburn-1.example", "iad..example", "iad.tenant.example"} {
		c := operatorChannelFixture()
		c.TopicID = "ocid1.onstopic.oc1." + suffix
		c.SubscriptionID = "ocid1.onssubscription.oc1." + suffix
		i, err := NewOperatorIntent(strings.Repeat("a", 64), "installation-1", "test-1", c)
		if err != nil || i.TopicID != c.TopicID || i.SubscriptionID != c.SubscriptionID {
			t.Fatal("valid regional reference was changed/rejected", err)
		}
	}
}
