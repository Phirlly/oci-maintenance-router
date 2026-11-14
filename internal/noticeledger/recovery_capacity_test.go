package noticeledger

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryCapacityReservesTerminalResultsAndInvalidation(t *testing.T) {
	manual, _, ref, at := claimedRetry(t, false)
	approved, _, _ := approvedRecord(t, true)
	for _, r := range []Record{manual, approved} {
		full := fullSourceRecord(t, r)
		env, id := full.Identity()
		over := changedWire(t, full, func(w *wireRecord) { *w.Revisions[0].Source.Reference += "x" })
		bad, err := Decode(over, env, id)
		rejected(t, bad, err)
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "too_large" {
			t.Fatal("one-byte reserve overflow admitted", err)
		}
		var next Record
		if r.Snapshot().Recovery.Actions[0].Status == RecoveryConsumed {
			for _, status := range []int{0, 202, 400, 429, 503} {
				next, err = full.Finish(ref, status, at, time.Time{})
				if err != nil {
					t.Fatal("reserved terminal cannot fit", err)
				}
				reload(t, next)
			}
			next, err = full.Recover(at.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			reload(t, next)
		} else {
			next, err = full.Hold(delivery.ReasonConfigurationChanged, clock().Add(time.Second))
			if err != nil {
				t.Fatal("reserved invalidation cannot fit", err)
			}
			reload(t, next)
		}
	}
}

func TestRecoveryCapacityFailureCannotLoseLegacyReceiptsOrConsumeRequest(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	sending, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	full := fullSourceRecord(t, sending)
	before := encoded(t, full)
	bad, err := full.MigrateRecovery(clock())
	rejected(t, bad, err)
	if !bytes.Equal(before, encoded(t, full)) {
		t.Fatal("failed migration changed legacy state")
	}
	accepted, err := full.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	reload(t, accepted)
	expired, err := full.Recover(clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	reload(t, expired)
	review := fullSourceRecord(t, retryReady(t, true))
	q := retryRequest(t, review)
	bad, action, newRef, err := review.RetryRecovery(q, retryContext(review, clock().Add(48*time.Hour)))
	rejected(t, bad, err)
	if action.Status != "" || newRef != (delivery.AttemptRef{}) || len(review.Snapshot().Recovery.Actions) != 0 {
		t.Fatal("capacity failure consumed request")
	}
}
