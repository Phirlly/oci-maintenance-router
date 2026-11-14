package reconciliation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func operatorCorruption(t *testing.T, r OperatorChallenge, change func(*operatorChallengeData)) {
	t.Helper()
	d := *r.data
	change(&d)
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	i := operatorIntentFixture(t)
	got, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err == nil || got.Snapshot().State != "" {
		t.Fatal("corrupt canonical state accepted")
	}
}

func TestOperatorChallengeCanonicalCodecRevalidatesIdentityAndFixedMessage(t *testing.T) {
	r := operatorChallengeFixture(t)
	for _, change := range []func(*operatorChallengeData){
		func(d *operatorChallengeData) { d.SchemaVersion = 0 }, func(d *operatorChallengeData) { d.Intent.Protocol = "HTTPS" },
		func(d *operatorChallengeData) { d.OwnerToken = "wrong" }, func(d *operatorChallengeData) { d.ClaimToken = "wrong" },
		func(d *operatorChallengeData) { d.IntentDigest = strings.Repeat("f", 64) }, func(d *operatorChallengeData) { d.PayloadDigest = strings.Repeat("f", 64) },
		func(d *operatorChallengeData) {
			d.Payload = strings.Replace(d.Payload, "operator test", "maintenance event", 1)
			d.PayloadDigest = operatorDigest(d.Payload)
		},
		func(d *operatorChallengeData) {
			d.Payload = strings.Replace(d.Payload, "OMR-OP-", "OTHER-", 1)
			d.PayloadDigest = operatorDigest(d.Payload)
		},
		func(d *operatorChallengeData) {
			d.Payload = strings.Replace(d.Payload, `"body":`, `"endpoint":"private@example.invalid","body":`, 1)
			d.PayloadDigest = operatorDigest(d.Payload)
		},
	} {
		operatorCorruption(t, r, change)
	}
}

func TestOperatorChallengeCanonicalCodecRevalidatesStateAndClock(t *testing.T) {
	r := operatorChallengeFixture(t)
	c := operatorClaimFixture()
	accepted, _ := r.Finish(operatorReference(c), 200, "message", c.Now)
	unknown, _ := r.Finish(operatorReference(c), 0, "", c.Now)
	unverified, _ := r.Finish(operatorReference(c), 200, "", c.Now)
	response, _ := r.Finish(operatorReference(c), 503, "", c.Now)
	expired, _ := r.Expire(c.ExpiresAt)
	for _, tc := range []struct {
		record OperatorChallenge
		change func(*operatorChallengeData)
	}{
		{r, func(d *operatorChallengeData) { d.StartedAt = "bad" }}, {r, func(d *operatorChallengeData) { d.ExpiresAt = "bad" }}, {r, func(d *operatorChallengeData) { d.UpdatedAt = "bad" }},
		{r, func(d *operatorChallengeData) { d.ExpiresAt = d.StartedAt }}, {r, func(d *operatorChallengeData) {
			d.ExpiresAt = c.ExpiresAt.Add(time.Millisecond).Format(leaseTimeFormat)
		}},
		{r, func(d *operatorChallengeData) { d.UpdatedAt = c.Now.Add(-time.Millisecond).Format(leaseTimeFormat) }},
		{r, func(d *operatorChallengeData) { d.State = "RETRY_WAIT" }}, {r, func(d *operatorChallengeData) { d.HTTPStatus = 200 }}, {r, func(d *operatorChallengeData) { d.Reason = "response" }},
		{r, func(d *operatorChallengeData) { d.Ambiguous = true }}, {r, func(d *operatorChallengeData) { d.MessageID = "message" }},
		{r, func(d *operatorChallengeData) { v := d.StartedAt; d.AcceptedAt = &v }}, {r, func(d *operatorChallengeData) { d.UpdatedAt = c.Now.Add(time.Millisecond).Format(leaseTimeFormat) }},
		{accepted, func(d *operatorChallengeData) { d.AcceptedAt = nil }}, {accepted, func(d *operatorChallengeData) { d.HTTPStatus = 202 }},
		{accepted, func(d *operatorChallengeData) { d.MessageID = "" }}, {accepted, func(d *operatorChallengeData) { d.MessageID = "bad\n" }},
		{accepted, func(d *operatorChallengeData) { d.Ambiguous = true }}, {accepted, func(d *operatorChallengeData) { d.Reason = "expired" }},
		{accepted, func(d *operatorChallengeData) { v := d.ExpiresAt; d.AcceptedAt = &v }},
		{accepted, func(d *operatorChallengeData) { d.UpdatedAt = d.ExpiresAt; v := d.UpdatedAt; d.AcceptedAt = &v }},
		{unknown, func(d *operatorChallengeData) { d.Reason = "private" }}, {unknown, func(d *operatorChallengeData) { d.HTTPStatus = 400 }},
		{unknown, func(d *operatorChallengeData) { d.Ambiguous = false }}, {unknown, func(d *operatorChallengeData) { d.UpdatedAt = d.ExpiresAt }},
		{unverified, func(d *operatorChallengeData) { d.HTTPStatus = 202 }}, {unverified, func(d *operatorChallengeData) { d.Ambiguous = false }},
		{unverified, func(d *operatorChallengeData) { d.UpdatedAt = d.ExpiresAt }},
		{response, func(d *operatorChallengeData) { d.HTTPStatus = 200 }}, {response, func(d *operatorChallengeData) { d.HTTPStatus = 99 }}, {response, func(d *operatorChallengeData) { d.HTTPStatus = 600 }},
		{response, func(d *operatorChallengeData) { d.Ambiguous = false }}, {response, func(d *operatorChallengeData) { d.UpdatedAt = d.ExpiresAt }},
		{response, func(d *operatorChallengeData) { d.MessageID = "message" }}, {response, func(d *operatorChallengeData) { v := d.UpdatedAt; d.AcceptedAt = &v }},
		{expired, func(d *operatorChallengeData) { d.UpdatedAt = d.StartedAt }}, {expired, func(d *operatorChallengeData) { d.Ambiguous = false }}, {expired, func(d *operatorChallengeData) { d.HTTPStatus = 200 }},
	} {
		operatorCorruption(t, tc.record, tc.change)
	}
}
