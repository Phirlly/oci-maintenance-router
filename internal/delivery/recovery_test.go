package delivery_test

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryDistinguishesActiveExpiredAndExhausted(t *testing.T) {
	r, ref := sending(t)
	active, err := r.Recover(instant().Add(time.Minute - time.Millisecond))
	if err != nil || !reflect.DeepEqual(active.Batches(), r.Batches()) {
		t.Fatal("active attempt recovered")
	}
	expired, err := r.Recover(instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	b := expired.Batches()[0]
	if b.State != delivery.RetryWait || b.Reason != delivery.ReasonExpired || !b.Ambiguous || !b.NextAttemptAt.Equal(instant().Add(time.Minute)) || b.AttemptCount != 1 || b.OwnerToken != ref.OwnerToken {
		t.Fatal("expired attempt lost ambiguity")
	}
	for _, base := range []delivery.Record{r, expired} {
		held, err := base.Recover(instant().Add(24 * time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		b := held.Batches()[0]
		if b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonHorizon || !b.Ambiguous || !b.NextAttemptAt.IsZero() {
			t.Fatal("horizon failed")
		}
	}
	got, err := expired.Recover(instant())
	rejectedRecord(t, got, err)
	got, err = (delivery.Record{}).Recover(instant())
	rejectedRecord(t, got, err)
	got, err = r.Recover(zeroTime())
	rejectedRecord(t, got, err)
}

func TestHoldRetainsAssignmentAndCannotResume(t *testing.T) {
	r, _ := prepared(t, db)
	s, ref := sending(t)
	wait, err := s.Finish(ref, 0, instant(), instant())
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []delivery.Record{r, wait} {
		for _, reason := range []delivery.Reason{delivery.ReasonSourceChanged, delivery.ReasonSourceUnavailable, delivery.ReasonConfigurationChanged, delivery.ReasonSecretUnavailable, delivery.ReasonOperator} {
			held, err := base.Hold(ref.BatchID, reason, instant())
			if err != nil {
				t.Fatal(err)
			}
			b := held.Batches()[0]
			if b.State != delivery.ReviewRequired || b.Reason != reason || !b.NextAttemptAt.IsZero() || !reflect.DeepEqual(b.Message, base.Batches()[0].Message) {
				t.Fatal("hold changed message")
			}
			got, _, err := attempt(held, instant())
			rejectedRecord(t, got, err)
			recovered, err := held.Recover(instant())
			if err != nil || !reflect.DeepEqual(recovered.Batches(), held.Batches()) {
				t.Fatal("review automatically resumed")
			}
		}
	}
	for _, reason := range []delivery.Reason{"private-error", delivery.ReasonExpired, delivery.ReasonHorizon, ""} {
		got, err := r.Hold(ref.BatchID, reason, instant())
		rejectedRecord(t, got, err)
	}
	got, err := s.Hold(ref.BatchID, delivery.ReasonOperator, instant())
	rejectedRecord(t, got, err)
	got, err = r.Hold("missing", delivery.ReasonOperator, instant())
	rejectedRecord(t, got, err)
	got, err = (delivery.Record{}).Hold(ref.BatchID, delivery.ReasonOperator, instant())
	rejectedRecord(t, got, err)
	got, err = r.Hold(ref.BatchID, delivery.ReasonOperator, zeroTime())
	rejectedRecord(t, got, err)
}
