package reconciliation

import (
	"errors"
	"strings"
	"testing"
)

func TestOperatorChallengeRejectsMalformedChannel(t *testing.T) {
	i := operatorIntentFixture(t)
	for _, change := range []func(*OperatorChannel){
		func(c *OperatorChannel) { c.Protocol = "email" }, func(c *OperatorChannel) { c.Protocol = "HTTPS" },
		func(c *OperatorChannel) { c.TopicID = "private" }, func(c *OperatorChannel) { c.TopicID = "ocid1.onstopic.oc2.iad.other" },
		func(c *OperatorChannel) { c.TopicID = "ocid1.onstopic.oc1..other" }, func(c *OperatorChannel) { c.TopicID = "ocid1.onstopic.oc1.iad." + strings.Repeat("x", 4096) },
		func(c *OperatorChannel) { c.SubscriptionID = "ocid1.onstopic.oc1.iad.other" }, func(c *OperatorChannel) { c.SubscriptionID = "ocid1.onssubscription.oc1..other" },
		func(c *OperatorChannel) {
			c.SubscriptionID = "ocid1.onssubscription.oc1.iad." + strings.Repeat("x", 4096)
		},
		func(c *OperatorChannel) { c.Endpoint = "" }, func(c *OperatorChannel) { c.Endpoint = "private\r\n@example.invalid" },
		func(c *OperatorChannel) { c.Endpoint = " private@example.invalid" }, func(c *OperatorChannel) { c.Endpoint = "private\u00a0@example.invalid" },
		func(c *OperatorChannel) { c.Endpoint = string([]byte{255}) }, func(c *OperatorChannel) { c.Endpoint = strings.Repeat("a", 513) },
		func(c *OperatorChannel) { c.Endpoint = strings.Repeat("🙂", 513) },
	} {
		c := operatorChannelFixture()
		change(&c)
		got, err := NewOperatorIntent(i.EnvironmentID, i.InstallationID, i.RequestID, c)
		var detail *OperatorChallengeError
		if !errors.As(err, &detail) || detail.Code != "invalid_intent" || got != (OperatorIntent{}) {
			t.Fatal("malformed channel returned partial intent", err)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("input escaped diagnostics")
		}
	}
	for _, endpoint := range []string{strings.Repeat("a", 512), strings.Repeat("🙂", 512), "Case+tag@example.invalid"} {
		c := operatorChannelFixture()
		c.Endpoint = endpoint
		if _, err := NewOperatorIntent(i.EnvironmentID, i.InstallationID, i.RequestID, c); err != nil {
			t.Fatal("valid structural boundary rejected", err)
		}
	}
}

func TestOperatorChallengeValidatesLiteralIntent(t *testing.T) {
	for _, change := range []func(*OperatorIntent){
		func(i *OperatorIntent) { i.EnvironmentID = strings.Repeat("A", 64) }, func(i *OperatorIntent) { i.EnvironmentID = strings.Repeat("a", 63) },
		func(i *OperatorIntent) { i.InstallationID = "" }, func(i *OperatorIntent) { i.InstallationID = strings.Repeat("a", 129) },
		func(i *OperatorIntent) { i.RequestID = "../private" }, func(i *OperatorIntent) { i.RequestID = "🙂" }, func(i *OperatorIntent) { i.RequestID = strings.Repeat("a", 129) },
		func(i *OperatorIntent) { i.TopicID = "wrong" }, func(i *OperatorIntent) { i.SubscriptionID = "wrong" }, func(i *OperatorIntent) { i.Protocol = "HTTPS" },
		func(i *OperatorIntent) { i.EndpointDigest = "" }, func(i *OperatorIntent) { i.EndpointDigest = strings.Repeat("F", 64) },
	} {
		i := operatorIntentFixture(t)
		change(&i)
		r, err := NewOperatorChallenge(i, operatorClaimFixture())
		operatorRejected(t, r, err, "invalid_intent")
		if operatorChallengeFixture(t).Matches(i) {
			t.Fatal("malformed intent matched")
		}
	}
	for _, ids := range [][3]string{{"private", "installation-1", "test"}, {strings.Repeat("a", 64), "../private", "test"}, {strings.Repeat("a", 64), "installation-1", "private bad"}} {
		got, err := NewOperatorIntent(ids[0], ids[1], ids[2], operatorChannelFixture())
		if err == nil || got != (OperatorIntent{}) {
			t.Fatal("invalid lookup identity supplied intent")
		}
	}
}
