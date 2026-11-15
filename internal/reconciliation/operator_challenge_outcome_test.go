package reconciliation

import (
	"bytes"
	"testing"
	"time"
)

func TestOperatorChallengePublicationOutcomesAreTerminal(t *testing.T) {
	for _, tc := range []struct {
		status    int
		id        string
		state     OperatorChallengeState
		reason    string
		ambiguous bool
	}{
		{200, "550e8400-e29b-41d4-a716-446655440000", OperatorAccepted, "", false},
		{202, "message-accepted", OperatorAccepted, "", false},
		{200, "", OperatorReview, "response_unverified", true}, {0, "", OperatorReview, "outcome_unknown", true},
		{100, "", OperatorReview, "response", true}, {202, "", OperatorReview, "response", true}, {299, "", OperatorReview, "response", true},
		{300, "", OperatorReview, "response", false}, {400, "", OperatorReview, "response", false}, {429, "", OperatorReview, "response", false},
		{499, "", OperatorReview, "response", false}, {500, "", OperatorReview, "response", true}, {599, "", OperatorReview, "response", true},
	} {
		r := operatorChallengeFixture(t)
		c := operatorClaimFixture()
		at := c.Now.Add(time.Second)
		got, err := r.Finish(operatorReference(c), tc.status, tc.id, at)
		if err != nil {
			t.Fatal(err)
		}
		s := got.Snapshot()
		if s.State != tc.state || s.HTTPStatus != tc.status || s.MessageID != tc.id || s.Reason != tc.reason || s.Ambiguous != tc.ambiguous || !s.UpdatedAt.Equal(at) {
			t.Fatalf("wrong publication outcome: %+v", s)
		}
		if (tc.state == OperatorAccepted && !s.AcceptedAt.Equal(at)) || (tc.state != OperatorAccepted && !s.AcceptedAt.IsZero()) {
			t.Fatal("false acceptance timestamp")
		}
		if r.Snapshot().State != OperatorPublishing || s.Intent != r.Snapshot().Intent || !bytes.Equal(s.Payload, r.Snapshot().Payload) {
			t.Fatal("outcome changed original claim/message")
		}
		next, err := got.Finish(operatorReference(c), 200, tc.id, at)
		operatorRejected(t, next, err, "terminal")
		expired, err := got.Expire(c.ExpiresAt)
		if err != nil || expired.Snapshot().State != tc.state {
			t.Fatal("terminal state changed at expiry", err)
		}
		body, err := got.Encode()
		if err != nil {
			t.Fatal(err)
		}
		i := s.Intent
		loaded, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
		if err != nil || loaded.Snapshot().MessageID != tc.id || loaded.Snapshot().Reason != tc.reason || loaded.Snapshot().State != tc.state {
			t.Fatal("terminal evidence lost on restart", err)
		}
	}
}

func TestOperatorChallengeExpiryCannotAuthorizeRetry(t *testing.T) {
	r := operatorChallengeFixture(t)
	c := operatorClaimFixture()
	early, err := r.Expire(c.ExpiresAt.Add(-time.Millisecond))
	if err != nil || early.Snapshot().State != OperatorPublishing {
		t.Fatal("early expiry", err)
	}
	expired, err := r.Expire(c.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	s := expired.Snapshot()
	if s.State != OperatorReview || s.Reason != "expired" || !s.Ambiguous || s.HTTPStatus != 0 || s.MessageID != "" || !s.AcceptedAt.IsZero() {
		t.Fatal("expiry was not ambiguous review")
	}
	late, err := r.Finish(operatorReference(c), 200, "message", c.ExpiresAt)
	operatorRejected(t, late, err, "stale_claim")
	late, err = expired.Finish(operatorReference(c), 200, "message", c.ExpiresAt)
	operatorRejected(t, late, err, "terminal")
}
