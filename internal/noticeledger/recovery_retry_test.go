package noticeledger

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryRetryConsumesExactAttemptAndHistoryAtomically(t *testing.T) {
	for _, prior := range []bool{false, true} {
		r := retryReady(t, prior)
		q := retryRequest(t, r)
		before := encoded(t, r)
		b := r.Snapshot().Batches[0]
		at := clock().Add(48 * time.Hour)
		next, a, ref, err := r.RetryRecovery(q, retryContext(r, at))
		if err != nil {
			t.Fatal(err)
		}
		got := next.Snapshot().Batches[0]
		if !next.Follows(r) || a.Status != RecoveryConsumed || a.Attempt == nil || a.CompletedAt != nil || a.Reason != "" || a.RecordedAt != at.Format(timeFormat) || a.Attempt.BatchID != ref.BatchID || a.Attempt.OwnerToken != ref.OwnerToken || a.Attempt.AttemptCount != ref.AttemptCount || a.Attempt.ExpiresAt != at.Add(time.Minute).Format(timeFormat) || a.Ambiguous != prior {
			t.Fatal("request not bound to its exact proposed claim")
		}
		if got.State != delivery.Sending || got.AttemptKind != delivery.ManualAttempt || got.AttemptCount != b.AttemptCount+1 || !reflect.DeepEqual(got.Message, b.Message) || got.SecretVersion != b.SecretVersion || got.Ambiguous != b.Ambiguous {
			t.Fatal("manual claim changed saved identity/history")
		}
		if prior && !got.FirstAttemptAt.Equal(b.FirstAttemptAt) {
			t.Fatal("first attempt reset")
		}
		if !bytes.Equal(before, encoded(t, r)) {
			t.Fatal("proposal mutated predecessor")
		}
		reload(t, next)
		for _, record := range []Record{next, mustDecodeRecovery(t, next)} {
			repeat, saved, repeatedRef, err := record.RetryRecovery(q, delivery.AttemptContext{})
			if err != nil || repeat.Follows(record) || !bytes.Equal(encoded(t, repeat), encoded(t, record)) || !reflect.DeepEqual(saved, a) || repeatedRef != (delivery.AttemptRef{}) {
				t.Fatal("consumed request gained another attempt", err)
			}
		}
	}
}

func mustDecodeRecovery(t *testing.T, r Record) Record {
	t.Helper()
	env, id := r.Identity()
	got, err := Decode(encoded(t, r), env, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRecoveryRetryRejectsStaleReviewAndInvalidAttemptMetadata(t *testing.T) {
	r := retryReady(t, true)
	q := retryRequest(t, r)
	at := clock().Add(48 * time.Hour)
	stale := q
	stale.ReviewFingerprint = strings.Repeat("c", 64)
	next, a, ref, err := r.RetryRecovery(stale, retryContext(r, at))
	if err != nil || !next.Follows(r) || a.Status != RecoveryRejected || a.Reason != "stale_review" || ref != (delivery.AttemptRef{}) || !reflect.DeepEqual(next.Snapshot().Batches, r.Snapshot().Batches) {
		t.Fatal("stale action dispatched or lacked retained rejection", err)
	}
	reload(t, next)
	for _, change := range []func(*delivery.AttemptContext){
		func(c *delivery.AttemptContext) { c.OwnerToken = "" }, func(c *delivery.AttemptContext) { c.RoutingGeneration = "changed" },
		func(c *delivery.AttemptContext) { c.Now = time.Time{} }, func(c *delivery.AttemptContext) { c.Now = clock().Add(-time.Second) },
		func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now }, func(c *delivery.AttemptContext) { c.LeaseExpiresAt = c.Now.Add(601 * time.Second) },
	} {
		c := retryContext(r, at)
		change(&c)
		bad, a, ref, err := r.RetryRecovery(q, c)
		rejected(t, bad, err)
		if a.Status != "" || ref != (delivery.AttemptRef{}) {
			t.Fatal("invalid caller metadata consumed action")
		}
	}
	q.Kind = CloseBatch
	q.ComponentFingerprint = ""
	bad, _, _, err := r.RetryRecovery(q, retryContext(r, at))
	rejected(t, bad, err)
}
