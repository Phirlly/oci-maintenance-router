package delivery_test

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func upgraded(t *testing.T, r delivery.Record) delivery.Record {
	t.Helper()
	next, err := r.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestRecoveryMigrationPreservesEveryLegacyState(t *testing.T) {
	p, _ := prepared(t, db)
	s, ref := sending(t)
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
	expired, err := s.Recover(instant().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []delivery.Record{p, s, accepted, retry, held, expired} {
		before := encoded(t, old)
		if old.SchemaVersion() != 1 {
			t.Fatal("legacy format changed")
		}
		next := upgraded(t, old)
		if next.SchemaVersion() != 2 || !reflect.DeepEqual(next.Batches(), old.Batches()) || !bytes.Equal(before, encoded(t, old)) {
			t.Fatal("migration altered saved history or predecessor")
		}
		b := next.Batches()[0]
		want := delivery.AttemptKind("")
		if b.AttemptCount > 0 {
			want = delivery.AutomaticAttempt
		}
		if b.AttemptKind != want {
			t.Fatal("migration invented manual provenance")
		}
		restored(t, next)
		got, err := next.MigrateRecovery()
		rejectedRecord(t, got, err)
	}
	if (delivery.Record{}).SchemaVersion() != 0 {
		t.Fatal("zero record has a version")
	}
	got, err := (delivery.Record{}).MigrateRecovery()
	rejectedRecord(t, got, err)
}

func TestRecoveryMigrationRetainsActiveReceiptAndAutomaticExpiry(t *testing.T) {
	s, ref := sending(t)
	v2 := upgraded(t, s)
	accepted, err := v2.Finish(ref, 202, instant().Add(time.Millisecond), time.Time{})
	if err != nil || accepted.Batches()[0].State != delivery.APIAccepted {
		t.Fatal("migration lost active receipt", err)
	}
	restored(t, accepted)
	expired, err := v2.Recover(instant().Add(time.Minute))
	if err != nil || expired.Batches()[0].State != delivery.RetryWait || !expired.Batches()[0].Ambiguous {
		t.Fatal("automatic expiry changed", err)
	}
	restored(t, expired)
	got, _, err := attempt(expired, instant().Add(24*time.Hour))
	rejectedRecord(t, got, err)
}

func TestRecoveryPreparationRetainsVersionAndOwnsSnapshots(t *testing.T) {
	const otherDB = "ocid1.mysqldbsystem.oc1.iad.other"
	p, _ := prepared(t, db)
	next := upgraded(t, p)
	more := preparations(t, builder(t, nil), record(db, otherDB), otherDB)
	for _, old := range []delivery.Record{p, next} {
		added, err := old.Prepare(more...)
		if err != nil || added.SchemaVersion() != old.SchemaVersion() || len(added.Batches()) != 2 {
			t.Fatal("preparation changed version or lost batch", err)
		}
		for _, b := range added.Batches() {
			if b.AttemptKind != "" {
				t.Fatal("prepared batch has attempt provenance")
			}
		}
		snapshot := added.Batches()
		snapshot[0].AttemptKind = delivery.ManualAttempt
		if added.Batches()[0].AttemptKind != "" {
			t.Fatal("snapshot aliases provenance")
		}
		restored(t, added)
	}
}
