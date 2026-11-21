package noticeledger

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func migrateAnnouncements(t *testing.T, r Record) Record {
	t.Helper()
	at, err := time.Parse(timeFormat, r.Snapshot().UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	next, err := r.MigrateAnnouncements(at.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestAnnouncementMigrationPreservesExactWorkAndReview(t *testing.T) {
	p := recoveryRecord(t, prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...))
	s, ref := begin(t, p, p.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, err := s.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Finish(ref, 0, clock(), clock())
	if err != nil {
		t.Fatal(err)
	}
	approved, _, _ := approvedRecord(t, true)
	invalidated, err := approved.RecordSourceUnavailable(404, clock())
	if err != nil {
		t.Fatal(err)
	}
	manual, _, manualRef, at := claimedRetry(t, true)
	done, err := manual.Finish(manualRef, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := manual.StopUnsent(manualRef, at)
	if err != nil {
		t.Fatal(err)
	}
	closed, _, err := stopped.CloseRecovery(closeRequest(t, stopped, CloseBatch, manualRef.BatchID), at)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []Record{recoveryRecord(t, observed(t, source(db))), p, s, accepted, retry, approved, invalidated, manual, done, closed} {
		before := encoded(t, old)
		v := old.Snapshot().Revisions[0]
		fingerprint := reviewHash(t, old, ApproveRevision, v.Digest)
		next := migrateAnnouncements(t, old)
		if !next.Follows(old) || next.Snapshot().SchemaVersion != 4 {
			t.Fatal("not one explicit successor")
		}
		want := bytes.Replace(before, []byte(`"schemaVersion":3`), []byte(`"schemaVersion":4`), 1)
		want = bytes.Replace(want, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":3`), 1)
		want = bytes.Replace(want, []byte(`"updatedAt":"`+old.Snapshot().UpdatedAt+`"`), []byte(`"updatedAt":"`+next.Snapshot().UpdatedAt+`"`), 1)
		if !bytes.Equal(want, encoded(t, next)) || !bytes.Equal(before, encoded(t, old)) {
			t.Fatal("migration rewrote retained work or predecessor")
		}
		if fingerprint != reviewHash(t, next, ApproveRevision, v.Digest) || maximumSize(t, old) != maximumSize(t, next) {
			t.Fatal("review or reserved size changed")
		}
		for _, action := range old.Snapshot().Recovery.Actions {
			got, exists, err := next.LookupRecovery(action.Request)
			if err != nil || !exists || !reflect.DeepEqual(got, action) {
				t.Fatal("immutable intent changed", err)
			}
		}
		reload(t, next)
	}
}

func TestAnnouncementMigrationRemainsExplicitAndClockBounded(t *testing.T) {
	old := recoveryRecord(t, observed(t, source(db)))
	up := migrateAnnouncements(t, old)
	for _, r := range []Record{{}, legacyRecord(t, observed(t, source(db))), observed(t, source(db)), up} {
		bad, err := r.MigrateAnnouncements(clock())
		rejected(t, bad, err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Millisecond), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		bad, err := old.MigrateAnnouncements(at)
		rejected(t, bad, err)
	}
	if observed(t, source(db)).Snapshot().SchemaVersion != 2 {
		t.Fatal("ordinary observation adopted4")
	}
	p := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	s, ref := begin(t, recoveryRecord(t, p), p.Snapshot().Batches[0].Message.BatchID, clock())
	up = migrateAnnouncements(t, s)
	next, err := up.Finish(ref, 202, clock().Add(time.Millisecond), time.Time{})
	if err != nil || next.Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("migration lost live attempt", err)
	}
	reload(t, next)
	expired, err := up.Recover(clock().Add(time.Minute))
	if err != nil || expired.Snapshot().Batches[0].State != delivery.RetryWait || !expired.Snapshot().Batches[0].Ambiguous {
		t.Fatal("migration lost expiry", err)
	}
	reload(t, expired)
}
