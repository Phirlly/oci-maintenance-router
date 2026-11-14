package reconciliation

import (
	"testing"
	"time"
)

func TestDiagnosticOutcomesAreTerminalWithoutRetry(t *testing.T) {
	for _, status := range []int{0, 100, 200, 201, 202, 204, 301, 400, 401, 404, 408, 429, 499, 500, 503, 599} {
		r := diagnosticFixture(t)
		c := diagnosticClaimFixture()
		at := c.Now.Add(time.Second)
		got, err := r.Finish(diagnosticReference(c), status, at)
		if err != nil {
			t.Fatal(err)
		}
		s := got.Snapshot()
		want, reason, ambiguous := DiagnosticReview, "response", status == 0 || status >= 500
		if status == 0 {
			reason = "outcome_unknown"
		}
		if status == 202 {
			want, reason, ambiguous = DiagnosticAccepted, "", false
		}
		if s.State != want || s.HTTPStatus != status || s.Reason != reason || s.Ambiguous != ambiguous || !s.UpdatedAt.Equal(at) || s.SecretVersion != 7 || r.Snapshot().State != DiagnosticSending {
			t.Fatalf("wrong terminal status%d: %+v", status, s)
		}
		if status == 202 && !s.AcceptedAt.Equal(at) || status != 202 && !s.AcceptedAt.IsZero() {
			t.Fatal("acceptance time does not match202")
		}
		again, err := got.Finish(diagnosticReference(c), 202, at)
		diagnosticRejected(t, again, err, "terminal")
		expired, err := got.Expire(c.ExpiresAt)
		if err != nil || expired.Snapshot().State != want || expired.Snapshot().HTTPStatus != status {
			t.Fatal("terminal record changed at expiry", err)
		}
	}
}

func TestDiagnosticExpiryRequiresReviewAndCannotResume(t *testing.T) {
	r := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	before, err := r.Expire(c.ExpiresAt.Add(-time.Millisecond))
	if err != nil || before.Snapshot().State != DiagnosticSending {
		t.Fatal("early expiry", err)
	}
	after, err := r.Expire(c.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	s := after.Snapshot()
	if s.State != DiagnosticReview || s.Reason != "expired" || !s.Ambiguous || s.HTTPStatus != 0 || !s.UpdatedAt.Equal(c.ExpiresAt) {
		t.Fatal("expired claim was not ambiguous review")
	}
	late, err := r.Finish(diagnosticReference(c), 202, c.ExpiresAt)
	diagnosticRejected(t, late, err, "stale_claim")
	late, err = after.Finish(diagnosticReference(c), 202, c.ExpiresAt)
	diagnosticRejected(t, late, err, "terminal")
}
