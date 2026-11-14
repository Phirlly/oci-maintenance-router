package delivery_test

import (
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func TestStopUnsentPreservesExactAttemptAndPriorAmbiguity(t *testing.T) {
	for _, priorAmbiguity := range []bool{false, true} {
		r, ref := sending(t)
		at := instant()
		if priorAmbiguity {
			var err error
			r, err = r.Finish(ref, 0, at, at.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			at = at.Add(time.Second)
			r, ref, err = attempt(r, at)
			if err != nil {
				t.Fatal(err)
			}
		}
		before := r.Batches()[0]
		got, err := r.StopUnsent(ref, at)
		if err != nil {
			t.Fatal(err)
		}
		after := got.Batches()[0]
		want := before
		want.State, want.Reason, want.UpdatedAt = delivery.ReviewRequired, delivery.ReasonOperator, at
		if !reflect.DeepEqual(after, want) || after.Ambiguous != priorAmbiguity {
			t.Fatal("unsent review changed attempt identity/history")
		}
		body, err := got.Encode()
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := delivery.DecodeRecord(body, after.Message.EnvironmentID, after.Message.Revision.NoticeID)
		if err != nil || !reflect.DeepEqual(loaded.Batches(), got.Batches()) {
			t.Fatal("review did not survive existing codec", err)
		}
		if !reflect.DeepEqual(r.Batches()[0], before) {
			t.Fatal("input mutated")
		}
		bad, err := got.StopUnsent(ref, at)
		rejectedRecord(t, bad, err)
	}
}

func TestStopUnsentRejectsForeignExpiredAndTerminalAttempts(t *testing.T) {
	r, ref := sending(t)
	for _, wrong := range []delivery.AttemptRef{{}, {BatchID: ref.BatchID, OwnerToken: "other", AttemptCount: ref.AttemptCount}, {BatchID: ref.BatchID, OwnerToken: ref.OwnerToken, AttemptCount: ref.AttemptCount + 1}} {
		got, err := r.StopUnsent(wrong, instant())
		rejectedRecord(t, got, err)
	}
	for _, at := range []time.Time{{}, instant().Add(-time.Millisecond), instant().Add(time.Minute)} {
		got, err := r.StopUnsent(ref, at)
		rejectedRecord(t, got, err)
	}
	prepared, _ := prepared(t, db)
	accepted, err := r.Finish(ref, 202, instant(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []delivery.Record{{}, prepared, accepted} {
		got, err := record.StopUnsent(ref, instant())
		rejectedRecord(t, got, err)
	}
}
