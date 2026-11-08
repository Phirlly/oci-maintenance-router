package noticeledger

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func begin(t *testing.T, r Record, id string, at time.Time) (Record, delivery.AttemptRef) {
	t.Helper()
	var generation string
	for _, b := range r.Snapshot().Batches {
		if b.Message.BatchID == id {
			generation = b.Message.RoutingGeneration
		}
	}
	n, ref, e := r.Begin(id, delivery.AttemptContext{OwnerToken: "worker", RoutingGeneration: generation, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	return n, ref
}
func TestReceiptUpdatesOnlyExactBatchMembers(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db, otherDB)
	p := preparations(t, src, db)
	r = prepare(t, r, append(p, preparations(t, src, otherDB)...)...)
	sending, ref := begin(t, r, p[0].Draft.Snapshot().BatchID, clock())
	accepted, e := sending.Finish(ref, 202, clock(), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	s := accepted.Snapshot()
	if s.Revisions[0].Resources[0].Disposition != "API_ACCEPTED" || s.Revisions[0].Resources[1].Disposition != "ASSIGNED" {
		t.Fatal("receipt changed unrelated membership")
	}
	if r.Snapshot().Revisions[0].Resources[0].Disposition != "ASSIGNED" {
		t.Fatal("receipt mutated predecessor")
	}
	bad, e := accepted.Finish(ref, 202, clock(), time.Time{})
	rejected(t, bad, e)
	if !accepted.Follows(sending) || accepted.Follows(r) || accepted.Follows(Record{}) || (Record{}).Follows(r) {
		t.Fatal("predecessor binding is not exact")
	}
}

func TestRecoveryRetainsRetryBytesAndMarksAmbiguousOutcome(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	s, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	recovered, e := s.Recover(clock().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	b := recovered.Snapshot().Batches[0]
	if b.State != delivery.RetryWait || !b.Ambiguous || !bytes.Equal(b.Message.PayloadWithoutRoutingKey, r.Snapshot().Batches[0].Message.PayloadWithoutRoutingKey) || b.SecretVersion != 7 {
		t.Fatal("recovery lost original delivery or uncertainty")
	}
	bad, e := recovered.Finish(ref, 202, clock().Add(time.Minute), time.Time{})
	rejected(t, bad, e)
	_, _ = begin(t, recovered, b.Message.BatchID, clock().Add(time.Minute))
}

func TestTransitionsRejectInvalidRecordsTimesAndAttempts(t *testing.T) {
	r := observed(t, source(db), db)
	n, _, e := r.Begin("missing", delivery.AttemptContext{Now: clock()})
	rejected(t, n, e)
	n, e = r.Finish(delivery.AttemptRef{}, 202, clock(), time.Time{})
	rejected(t, n, e)
	n, e = (Record{}).Recover(clock())
	rejected(t, n, e)
	n, e = r.Recover(time.Time{})
	rejected(t, n, e)
	n, e = r.Hold(delivery.Reason("private"), clock())
	rejected(t, n, e)
	n, e = (Record{}).Hold(delivery.ReasonOperator, clock())
	rejected(t, n, e)
	n, e = r.Hold(delivery.ReasonOperator, clock().Add(-time.Second))
	rejected(t, n, e)
}
