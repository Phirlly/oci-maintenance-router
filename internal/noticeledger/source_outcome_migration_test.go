package noticeledger

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func legacyRecord(t *testing.T, r Record) Record {
	t.Helper()
	env, id := r.Identity()
	b := changedWire(t, r, func(w *wireRecord) { w.SchemaVersion, w.SourceOutcome = 1, nil })
	next, err := Decode(b, env, id)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func nestedDelivery(t *testing.T, r Record) []byte {
	t.Helper()
	var w struct{ Delivery json.RawMessage }
	if err := json.Unmarshal(encoded(t, r), &w); err != nil {
		t.Fatal(err)
	}
	return w.Delivery
}

func TestLegacyGoldenRemainsCanonicalAndUnmigrated(t *testing.T) {
	b, err := os.ReadFile("testdata/source-only.json")
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.TrimSpace(b)
	r, err := Decode(b, "c081fc9b5129be4e85c6039722858f2454d79fcd9219504f473d53dffd712378", noticeID)
	if err != nil || !bytes.Equal(b, encoded(t, r)) || r.Snapshot().SchemaVersion != 1 || r.Snapshot().SourceOutcome != nil {
		t.Fatal("legacy bytes changed or implicitly migrated", err)
	}
	reload(t, r)
}

func TestExplicitSourceMigrationPreservesAllLegacyDeliveryStates(t *testing.T) {
	src := source(db)
	prepared := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	sending, ref := begin(t, prepared, prepared.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, err := sending.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := sending.Finish(ref, 0, clock(), clock())
	if err != nil {
		t.Fatal(err)
	}
	held, err := ambiguous.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []Record{observed(t, src), prepared, sending, accepted, ambiguous, held} {
		old := legacyRecord(t, r)
		reload(t, old)
		before := encoded(t, old)
		at := clock().Add(time.Second)
		next, err := old.MigrateSourceOutcome(at)
		if err != nil {
			t.Fatal(err)
		}
		s := next.Snapshot()
		want := SourceOutcome{State: SourceUnknown, RecordedAt: at.Format(timeFormat)}
		if s.SchemaVersion != 2 || s.SourceOutcome == nil || *s.SourceOutcome != want || !next.Follows(old) || next.Follows(r) {
			t.Fatal("migration inferred availability or lost immediate parent")
		}
		if !bytes.Equal(before, encoded(t, old)) || !bytes.Equal(nestedDelivery(t, old), nestedDelivery(t, next)) || !reflect.DeepEqual(old.Snapshot().Revisions, s.Revisions) || s.HoldReason != old.Snapshot().HoldReason {
			t.Fatal("migration changed history or delivery bytes")
		}
		reload(t, next)
	}
}

func TestSourceMigrationIsExplicitAndRejectsInvalidTransitions(t *testing.T) {
	r := observed(t, source(db))
	old := legacyRecord(t, r)
	before := encoded(t, old)
	for _, bad := range []Record{{}, r} {
		n, err := bad.MigrateSourceOutcome(clock())
		rejected(t, n, err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Second), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		n, err := old.MigrateSourceOutcome(at)
		rejected(t, n, err)
	}
	n, err := observer(t, nil).Observe(old, normalized(t, source(db)), false, nil, clock())
	rejected(t, n, err)
	n, err = captured(t, normalized(t, source(db))).Apply(old, clock())
	rejected(t, n, err)
	n, err = old.RecordSourceUnavailable(404, clock())
	rejected(t, n, err)
	if !bytes.Equal(before, encoded(t, old)) {
		t.Fatal("rejected migration or observation changed legacy state")
	}
}

func TestLegacyPreparationAttemptAndHoldRemainUsable(t *testing.T) {
	src := source(db)
	r := legacyRecord(t, observed(t, src, db))
	r = prepare(t, r, preparations(t, src, db)...)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	held, err := r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := held.Finish(ref, 202, clock(), time.Time{})
	if err != nil || accepted.Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("legacy active result blocked", err)
	}
	for _, next := range []Record{r, held, accepted} {
		if next.Snapshot().SchemaVersion != 1 || next.Snapshot().SourceOutcome != nil {
			t.Fatal("legacy operation implicitly migrated")
		}
		reload(t, next)
	}
}
