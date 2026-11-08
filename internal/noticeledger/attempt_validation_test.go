package noticeledger

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
)

func TestInvalidAttemptInputsDoNotProduceWholeNoticeCandidates(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	b := r.Snapshot().Batches[0]
	for _, c := range []delivery.AttemptContext{{Now: time.Time{}}, {Now: clock(), LeaseExpiresAt: clock().Add(time.Minute), OwnerToken: "wrong owner", RoutingGeneration: b.Message.RoutingGeneration}, {Now: clock(), LeaseExpiresAt: clock(), OwnerToken: "worker", RoutingGeneration: b.Message.RoutingGeneration}} {
		n, ref, e := r.Begin(b.Message.BatchID, c)
		rejected(t, n, e)
		if ref.BatchID != "" {
			t.Fatal("invalid attempt returned reference")
		}
	}
	s, ref := begin(t, r, b.Message.BatchID, clock())
	n, e := s.Finish(ref, 202, clock().Add(-time.Second), time.Time{})
	rejected(t, n, e)
	n, e = s.Finish(ref, 999, clock(), time.Time{})
	rejected(t, n, e)
	n, e = (Record{}).Finish(ref, 202, clock(), time.Time{})
	rejected(t, n, e)
	if _, e := New(settings(nil), catalog(), configuration.LedgerLocation{}); e == nil {
		t.Fatal("invalid trusted location accepted")
	}
}

func TestOverlappingNewPreparationsFailWithoutPartialAssignments(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db, otherDB)
	values := append(preparations(t, src, db), preparations(t, src, db, otherDB)...)
	n, e := r.Prepare(clock(), values...)
	rejected(t, n, e)
	if len(r.Snapshot().Batches) != 0 {
		t.Fatal("overlapping preparation mutated input")
	}
}
