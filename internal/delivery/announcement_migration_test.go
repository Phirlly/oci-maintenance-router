package delivery_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestAnnouncementMigrationPreservesRecoveryStatesAndActiveReceipts(t *testing.T) {
	p, _ := prepared(t, db)
	p = upgraded(t, p)
	s, ref, err := attempt(p, instant())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.Finish(ref, 202, instant(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Finish(ref, 0, instant(), instant())
	if err != nil {
		t.Fatal(err)
	}
	held, err := p.Hold(ref.BatchID, delivery.ReasonOperator, instant())
	if err != nil {
		t.Fatal(err)
	}
	closed, err := held.Close(ref.BatchID, instant())
	if err != nil {
		t.Fatal(err)
	}
	manual, mref, err := held.BeginManual(ref.BatchID, delivery.AttemptContext{OwnerToken: "manual", RoutingGeneration: p.Batches()[0].Message.RoutingGeneration, Now: instant(), LeaseExpiresAt: instant().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := manual.Recover(instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []delivery.Record{p, s, accepted, retry, held, closed, manual, expired} {
		before := encoded(t, old)
		next, err := old.MigrateAnnouncements()
		if err != nil {
			t.Fatal(err)
		}
		if next.SchemaVersion() != 3 || !reflect.DeepEqual(old.Batches(), next.Batches()) || !bytes.Equal(encoded(t, old), before) {
			t.Fatal("migration changed history/predecessor")
		}
		a, _ := old.ReservedBytes()
		b, _ := next.ReservedBytes()
		if a != b {
			t.Fatal("reservation changed")
		}
		restored(t, next)
		got, err := next.MigrateAnnouncements()
		rejectedRecord(t, got, err)
	}
	for _, tc := range []struct {
		record  delivery.Record
		ref     delivery.AttemptRef
		expired delivery.State
	}{{s, ref, delivery.RetryWait}, {manual, mref, delivery.ReviewRequired}} {
		next, err := tc.record.MigrateAnnouncements()
		if err != nil {
			t.Fatal(err)
		}
		got, err := next.Finish(tc.ref, 202, instant().Add(time.Second), time.Time{})
		if err != nil || got.Batches()[0].State != delivery.APIAccepted {
			t.Fatal("late receipt lost", err)
		}
		restored(t, got)
		got, err = next.Recover(instant().Add(time.Minute))
		if err != nil || got.Batches()[0].State != tc.expired || !got.Batches()[0].Ambiguous {
			t.Fatal("expiry changed", err)
		}
		restored(t, got)
	}
	legacy, _ := prepared(t, db)
	for _, invalid := range []delivery.Record{{}, legacy} {
		got, err := invalid.MigrateAnnouncements()
		rejectedRecord(t, got, err)
	}
}
