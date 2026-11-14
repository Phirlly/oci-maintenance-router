package noticeledger

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func recoveryRecord(t *testing.T, r Record) Record {
	t.Helper()
	at := clock()
	if r.data != nil {
		at, _ = time.Parse(timeFormat, r.data.UpdatedAt)
	}
	next, err := r.MigrateRecovery(at)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestRecoveryMigrationPreservesNoticeHistoryAndDelivery(t *testing.T) {
	p := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	s, ref := begin(t, p, p.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, err := s.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Finish(ref, 0, clock(), clock())
	if err != nil {
		t.Fatal(err)
	}
	held, err := p.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.Recover(clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	unavailable, err := p.RecordSourceUnavailable(404, clock())
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := legacyRecord(t, observed(t, source(db))).MigrateSourceOutcome(clock())
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []Record{observed(t, source(db)), p, s, accepted, retry, held, expired, unavailable, unknown} {
		before := encoded(t, old)
		view := old.Snapshot()
		next := recoveryRecord(t, old)
		got := next.Snapshot()
		if got.SchemaVersion != 3 || got.Recovery == nil || got.Recovery.Actions == nil || len(got.Recovery.Actions) != 0 || !next.Follows(old) {
			t.Fatal("migration did not produce one explicit action-aware successor")
		}
		if !reflect.DeepEqual(view.Revisions, got.Revisions) || !reflect.DeepEqual(view.Batches, got.Batches) || !reflect.DeepEqual(view.SourceOutcome, got.SourceOutcome) || view.HoldReason != got.HoldReason {
			t.Fatal("migration altered retained history")
		}
		if !bytes.Equal(before, encoded(t, old)) || old.Snapshot().Recovery != nil {
			t.Fatal("migration mutated predecessor")
		}
		if len(got.Batches) > 0 {
			env, id := next.Identity()
			d, err := delivery.DecodeRecord(nestedDelivery(t, next), env, id)
			if err != nil || d.SchemaVersion() != 2 {
				t.Fatal("wrong nested format", err)
			}
		}
		reload(t, next)
	}
}

func TestRecoveryMigrationIsExplicitAndKeepsLegacyOutcomes(t *testing.T) {
	p := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	s, ref := begin(t, p, p.Snapshot().Batches[0].Message.BatchID, clock())
	up := recoveryRecord(t, s)
	accepted, err := up.Finish(ref, 202, clock(), time.Time{})
	if err != nil || accepted.Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal(err)
	}
	reload(t, accepted)
	expired, err := up.Recover(clock().Add(time.Minute))
	if err != nil || expired.Snapshot().Batches[0].State != delivery.RetryWait {
		t.Fatal(err)
	}
	reload(t, expired)
	for _, bad := range []Record{{}, legacyRecord(t, p), up} {
		next, err := bad.MigrateRecovery(clock())
		rejected(t, next, err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Millisecond), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		next, err := p.MigrateRecovery(at)
		rejected(t, next, err)
	}
	if observed(t, source(db)).Snapshot().SchemaVersion != 2 {
		t.Fatal("ordinary observation adopted recovery")
	}
}
