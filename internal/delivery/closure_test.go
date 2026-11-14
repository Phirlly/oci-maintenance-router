package delivery_test

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func TestClosurePreservesHistoryWithoutClaimingAcceptance(t *testing.T) {
	p, values := prepared(t, db)
	s, ref := sending(t)
	wait, err := s.Finish(ref, 0, instant(), instant())
	if err != nil {
		t.Fatal(err)
	}
	m, mref, at := manualSending(t, true)
	manual, err := m.Finish(mref, 503, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	first, _, firstAt := manualSending(t, false)
	expired, err := first.Recover(firstAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []delivery.Record{upgraded(t, p), upgraded(t, wait), manualReady(t, false), manual, expired} {
		before := r.Batches()[0]
		old := encoded(t, r)
		closeAt := instant().Add(72 * time.Hour)
		next, err := r.Close(before.Message.BatchID, closeAt)
		if err != nil {
			t.Fatal(err)
		}
		want := before
		want.State, want.Reason, want.UpdatedAt, want.NextAttemptAt = delivery.Closed, delivery.ReasonOperator, closeAt, time.Time{}
		if !reflect.DeepEqual(next.Batches()[0], want) || !bytes.Equal(old, encoded(t, r)) {
			t.Fatal("closure changed history or claimed delivery")
		}
		restored(t, next)
		recovered, err := next.Recover(closeAt.Add(time.Hour))
		if err != nil || !reflect.DeepEqual(recovered.Batches(), next.Batches()) {
			t.Fatal("closed work changed on recovery")
		}
		repeated, err := next.Prepare(values...)
		if err != nil || !reflect.DeepEqual(repeated.Batches(), next.Batches()) {
			t.Fatal("preparation reopened closed assignment")
		}
		unassigned, err := next.Unassigned(before.Message.Revision, []string{db})
		if err != nil || len(unassigned) != 0 {
			t.Fatal("closure freed resource assignment", err)
		}
		got, _, err := attempt(next, closeAt)
		rejectedRecord(t, got, err)
		got, _, err = next.BeginManual(before.Message.BatchID, manualContext(next, closeAt))
		rejectedRecord(t, got, err)
		got, err = next.Hold(before.Message.BatchID, delivery.ReasonOperator, closeAt)
		rejectedRecord(t, got, err)
		got, err = next.Close(before.Message.BatchID, closeAt)
		rejectedRecord(t, got, err)
		ref := delivery.AttemptRef{BatchID: before.Message.BatchID, OwnerToken: before.OwnerToken, AttemptCount: before.AttemptCount}
		got, err = next.Finish(ref, 202, closeAt, time.Time{})
		rejectedRecord(t, got, err)
		got, err = next.StopUnsent(ref, closeAt)
		rejectedRecord(t, got, err)
	}
}

func TestClosureRejectsActiveAcceptedLegacyAndInvalidTargets(t *testing.T) {
	p, _ := prepared(t, db)
	s, ref := sending(t)
	accepted, err := s.Finish(ref, 202, instant(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	m, mref, at := manualSending(t, true)
	manualAccepted, err := m.Finish(mref, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Batches()[0].Message.BatchID
	for _, r := range []delivery.Record{{}, p, upgraded(t, s), upgraded(t, accepted), m, manualAccepted} {
		got, err := r.Close(id, instant().Add(72*time.Hour))
		rejectedRecord(t, got, err)
	}
	r := manualReady(t, false)
	got, err := r.Close("missing", instant())
	rejectedRecord(t, got, err)
	for _, at := range []time.Time{{}, instant().Add(-time.Millisecond), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		got, err := r.Close(id, at)
		rejectedRecord(t, got, err)
	}
}
