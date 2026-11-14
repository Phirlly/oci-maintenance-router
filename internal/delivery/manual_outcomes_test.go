package delivery_test

import (
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func TestManualOutcomesNeverScheduleAutomaticContinuation(t *testing.T) {
	for _, prior := range []bool{false, true} {
		for _, status := range []int{0, 100, 200, 202, 204, 301, 400, 401, 403, 429, 500, 503, 599} {
			r, ref, at := manualSending(t, prior)
			before := r.Batches()[0]
			next, err := r.Finish(ref, status, at, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			b := next.Batches()[0]
			want, reason := delivery.ReviewRequired, delivery.ReasonPermanent
			if status == 202 {
				want, reason = delivery.APIAccepted, ""
			} else if status == 0 {
				reason = delivery.ReasonNetwork
			} else if status == 429 || status >= 500 {
				reason = delivery.ReasonTransient
			}
			if b.State != want || b.Reason != reason || !b.NextAttemptAt.IsZero() || b.AttemptKind != delivery.ManualAttempt || b.Ambiguous != (prior || status == 0) || b.AttemptCount != before.AttemptCount || b.FirstAttemptAt != before.FirstAttemptAt || !reflect.DeepEqual(b.Message, before.Message) {
				t.Fatal("manual outcome continued or lost history", status)
			}
			restored(t, next)
			recovered, err := next.Recover(at.Add(48 * time.Hour))
			if err != nil || !reflect.DeepEqual(recovered.Batches(), next.Batches()) {
				t.Fatal("terminal manual outcome changed on recovery", err)
			}
			got, _, err := attempt(next, at)
			rejectedRecord(t, got, err)
		}
	}
}

func TestManualExpiryAndKnownUnsentRetainDistinctAmbiguity(t *testing.T) {
	for _, prior := range []bool{false, true} {
		r, ref, at := manualSending(t, prior)
		early, err := r.Recover(at.Add(time.Minute - time.Millisecond))
		if err != nil || early.Batches()[0].State != delivery.Sending {
			t.Fatal("manual attempt expired early")
		}
		expired, err := r.Recover(at.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		b := expired.Batches()[0]
		if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonExpired || !b.Ambiguous || !b.NextAttemptAt.IsZero() {
			t.Fatal("expired manual attempt became an automatic retry")
		}
		restored(t, expired)
		unsent, err := r.StopUnsent(ref, at)
		if err != nil {
			t.Fatal(err)
		}
		b = unsent.Batches()[0]
		if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonOperator || b.Ambiguous != prior || b.AttemptKind != delivery.ManualAttempt {
			t.Fatal("no-dispatch changed ambiguity/provenance")
		}
		restored(t, unsent)
		got, err := expired.Finish(ref, 202, at.Add(time.Minute), time.Time{})
		rejectedRecord(t, got, err)
	}
}

func TestManualFinishRejectsRetriesStaleReferencesAndInvalidTimes(t *testing.T) {
	r, ref, at := manualSending(t, true)
	for _, status := range []int{0, 202, 429, 503} {
		got, err := r.Finish(ref, status, at, at)
		rejectedRecord(t, got, err)
	}
	for _, status := range []int{-1, 99, 600} {
		got, err := r.Finish(ref, status, at, time.Time{})
		rejectedRecord(t, got, err)
	}
	for _, wrong := range []delivery.AttemptRef{{}, {BatchID: ref.BatchID, OwnerToken: "other", AttemptCount: ref.AttemptCount}, {BatchID: ref.BatchID, OwnerToken: ref.OwnerToken, AttemptCount: ref.AttemptCount - 1}} {
		got, err := r.Finish(wrong, 202, at, time.Time{})
		rejectedRecord(t, got, err)
	}
	for _, when := range []time.Time{{}, at.Add(-time.Millisecond), at.Add(time.Minute)} {
		got, err := r.Finish(ref, 202, when, time.Time{})
		rejectedRecord(t, got, err)
	}
}

func TestLaterManualAttemptPreservesHistoryAndRejectsOldResult(t *testing.T) {
	r, oldRef, at := manualSending(t, false)
	old := r.Batches()[0]
	expired, err := r.Recover(at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	later := at.Add(48 * time.Hour)
	next, ref, err := expired.BeginManual(oldRef.BatchID, manualContext(expired, later))
	if err != nil {
		t.Fatal(err)
	}
	b := next.Batches()[0]
	if b.AttemptCount != 2 || !b.FirstAttemptAt.Equal(old.FirstAttemptAt) || !b.Ambiguous || b.AttemptKind != delivery.ManualAttempt {
		t.Fatal("later manual attempt reset consumed history")
	}
	got, err := next.Finish(oldRef, 202, later, time.Time{})
	rejectedRecord(t, got, err)
	accepted, err := next.Finish(ref, 202, later, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	restored(t, accepted)
}
