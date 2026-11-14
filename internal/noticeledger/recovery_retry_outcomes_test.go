package noticeledger

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryRetryKeepsEveryOutcomeWithItsConsumedRequest(t *testing.T) {
	for _, prior := range []bool{false, true} {
		for _, mode := range []string{"accepted", "network", "transient", "rate", "permanent", "expired", "unsent"} {
			r, q, ref, at := claimedRetry(t, prior)
			var next Record
			var err error
			when := at
			reason := ""
			status := RecoveryReviewed
			ambiguous := prior
			switch mode {
			case "accepted":
				next, err = r.Finish(ref, 202, at, time.Time{})
				status = RecoveryAccepted
			case "network":
				next, err = r.Finish(ref, 0, at, time.Time{})
				reason = "network"
				ambiguous = true
			case "transient":
				next, err = r.Finish(ref, 503, at, time.Time{})
				reason = "transient"
			case "rate":
				next, err = r.Finish(ref, 429, at, time.Time{})
				reason = "transient"
			case "permanent":
				next, err = r.Finish(ref, 400, at, time.Time{})
				reason = "permanent"
			case "expired":
				when = at.Add(time.Minute)
				next, err = r.Recover(when)
				reason = "expired"
				ambiguous = true
			case "unsent":
				next, err = r.StopUnsent(ref, at)
				reason = "operator"
			}
			if err != nil {
				t.Fatal(mode, err)
			}
			a, found, err := next.LookupRecovery(q)
			if err != nil || !found || a.Status != status || a.Reason != reason || a.CompletedAt == nil || *a.CompletedAt != when.Format(timeFormat) || a.Ambiguous != ambiguous {
				t.Fatal("request outcome differs from its attempt", mode, a, err)
			}
			reload(t, next)
			repeat, saved, ref, err := next.RetryRecovery(q, delivery.AttemptContext{})
			if err != nil || repeat.Follows(next) || ref != (delivery.AttemptRef{}) || !reflect.DeepEqual(saved, a) {
				t.Fatal("outcome repeat gained dispatch", err)
			}
			if r.Snapshot().Recovery.Actions[0].Status != RecoveryConsumed {
				t.Fatal("outcome mutated claim")
			}
		}
	}
}

func TestRecoveryRetryPreservesEarlierResultAfterLaterAttempt(t *testing.T) {
	r, q, ref, at := claimedRetry(t, false)
	expired, err := r.Recover(at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := expired.LookupRecovery(q)
	if err != nil {
		t.Fatal(err)
	}
	laterQ := retryRequest(t, expired)
	laterQ.RequestID = "retry-2"
	laterAt := at.Add(48 * time.Hour)
	second, _, laterRef, err := expired.RetryRecovery(laterQ, retryContext(expired, laterAt))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := second.Finish(ref, 202, laterAt, time.Time{})
	rejected(t, bad, err)
	accepted, err := second.Finish(laterRef, 202, laterAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := accepted.LookupRecovery(q)
	if err != nil || !reflect.DeepEqual(old, first) {
		t.Fatal("later attempt overwrote earlier result")
	}
	if accepted.Snapshot().Batches[0].AttemptCount != 2 || !accepted.Snapshot().Batches[0].Ambiguous {
		t.Fatal("lifetime history lost")
	}
	reload(t, accepted)
}
