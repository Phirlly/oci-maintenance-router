package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"reflect"
	"testing"
	"time"
)

func TestHoldBatchPreservesIndependentAssignmentsAndReceipts(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db, otherDB)
	r = prepare(t, r, append(preparations(t, src, db), preparations(t, src, otherDB)...)...)
	before := r.Snapshot()
	id := before.Batches[0].Message.BatchID
	held, err := r.HoldBatch(id, delivery.ReasonSecretUnavailable, clock())
	if err != nil {
		t.Fatal(err)
	}
	if !held.Follows(r) || held.Snapshot().HoldReason != "" {
		t.Fatal("batch hold changed notice authority")
	}
	for i, b := range held.Snapshot().Batches {
		want := before.Batches[i]
		if b.Message.BatchID == id {
			want.State, want.Reason, want.UpdatedAt = delivery.ReviewRequired, delivery.ReasonSecretUnavailable, clock()
		}
		if !reflect.DeepEqual(b, want) {
			t.Fatal("batch hold changed unrelated state")
		}
	}
	env, notice := held.Identity()
	loaded, err := Decode(encoded(t, held), env, notice)
	if err != nil || !reflect.DeepEqual(loaded.Snapshot(), held.Snapshot()) {
		t.Fatal("hold restart", err)
	}
	if !reflect.DeepEqual(r.Snapshot(), before) {
		t.Fatal("input mutated")
	}
	peer := before.Batches[1].Message.BatchID
	sending, ref := begin(t, held, peer, clock())
	accepted, err := sending.Finish(ref, 202, clock(), time.Time{})
	if err != nil || accepted.Snapshot().Batches[1].State != delivery.APIAccepted {
		t.Fatal("independent batch blocked", err)
	}
}

func TestStopUnsentUpdatesWholeNoticeWithoutHoldingPeers(t *testing.T) {
	src := source(db, otherDB)
	r := prepare(t, observed(t, src, db, otherDB), append(preparations(t, src, db), preparations(t, src, otherDB)...)...)
	s, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	n, err := s.StopUnsent(ref, clock().Add(time.Millisecond))
	if err != nil || !n.Follows(s) || n.Snapshot().HoldReason != "" {
		t.Fatal("unsent transition", err)
	}
	if n.Snapshot().Batches[0].State != delivery.ReviewRequired || n.Snapshot().Batches[0].Ambiguous || n.Snapshot().Batches[1].State != delivery.Prepared {
		t.Fatal("wrong unsent dispositions")
	}
	env, id := n.Identity()
	if _, err := Decode(encoded(t, n), env, id); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{{}, clock().Add(-time.Second), clock().Add(time.Minute)} {
		n, err := s.StopUnsent(ref, at)
		rejected(t, n, err)
	}
	n, err = (Record{}).StopUnsent(ref, clock())
	rejected(t, n, err)
	n, err = s.StopUnsent(delivery.AttemptRef{}, clock())
	rejected(t, n, err)
	for _, item := range []struct {
		record Record
		id     string
		reason delivery.Reason
		at     time.Time
	}{
		{Record{}, ref.BatchID, delivery.ReasonOperator, clock()},
		{r, "missing", delivery.ReasonOperator, clock()},
		{r, ref.BatchID, "unknown", clock()},
		{r, ref.BatchID, delivery.ReasonOperator, time.Time{}},
		{s, ref.BatchID, delivery.ReasonOperator, clock()},
	} {
		n, err := item.record.HoldBatch(item.id, item.reason, item.at)
		rejected(t, n, err)
	}
}
