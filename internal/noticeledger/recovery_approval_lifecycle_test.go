package noticeledger

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryApprovedRetriesRetainAutomaticDeadlineAndDueTime(t *testing.T) {
	r, q, a := approvedRecord(t, false)
	id := a.BatchIDs[0]
	started, ref, err := r.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(r, id, clock()))
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := started.Finish(ref, 503, clock(), clock().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	reload(t, waiting)
	for _, at := range []time.Time{{}, clock(), clock().Add(24 * time.Hour)} {
		bad, _, err := waiting.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(waiting, id, at))
		rejected(t, bad, err)
	}
	next, _, err := waiting.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(waiting, id, clock().Add(time.Hour)))
	if err != nil || next.Snapshot().Batches[0].AttemptCount != 2 {
		t.Fatal("due approved retry failed", err)
	}
	saved, _, _ := next.LookupRecovery(q)
	if !reflect.DeepEqual(saved, a) {
		t.Fatal("automatic attempt rewrote approval")
	}
	reload(t, next)
}

func TestRecoveryApprovalPeerHoldAndCloseDoNotDisableOtherMembers(t *testing.T) {
	n := normalized(t, source(db, otherDB))
	found := lookups(db, otherDB)
	v := found[otherDB]
	v.Metadata.FreeformTags = map[string]string{"owner": "video"}
	found[otherDB] = v
	r, err := observer(t, nil).Observe(Record{}, n, false, found, clock())
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	r = recoveryRecord(t, r)
	q := approvalRequest(t, r, n.Key().Digest)
	r, a, err := observer(t, nil).ApproveRecovery(r, q, n, false, found, clock(), approvalDrafts(t, n, found, db, otherDB)...)
	if err != nil || len(a.BatchIDs) != 2 {
		t.Fatal(err)
	}
	first, peer := a.BatchIDs[0], a.BatchIDs[1]
	held, err := r.HoldBatch(first, delivery.ReasonSecretUnavailable, clock())
	if err != nil {
		t.Fatal(err)
	}
	closed, _, err := held.CloseRecovery(closeRequest(t, held, CloseBatch, first), clock())
	if err != nil {
		t.Fatal(err)
	}
	saved, _, _ := closed.LookupRecovery(q)
	if !reflect.DeepEqual(a, saved) {
		t.Fatal("peer closure invalidated the approval")
	}
	started, ref, err := closed.BeginApproved(q.RequestID, peer, q.ComponentFingerprint, approvedContext(closed, peer, clock()))
	if err != nil || ref.BatchID != peer {
		t.Fatal("independent peer blocked", err)
	}
	for _, b := range started.Snapshot().Batches {
		if b.Message.BatchID == first && b.State != delivery.Closed {
			t.Fatal("closed member reopened")
		}
	}
	reload(t, started)
}

func TestRecoveryApprovalSourceChangeIsStickyAndFingerprintBindsChain(t *testing.T) {
	r, q, a := approvedRecord(t, false)
	id := a.BatchIDs[0]
	s := source(db)
	s.ChainID = text("new-related-chain")
	changed, err := observer(t, nil).Observe(r, normalized(t, s), true, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	saved, _, err := changed.LookupRecovery(q)
	if err != nil || saved.InvalidatedAt == nil || saved.InvalidationReason != "source_changed" || changed.Snapshot().Batches[0].State != delivery.ReviewRequired {
		t.Fatal("source change retained approval", err)
	}
	_, err = changed.ReviewFingerprint(CloseRevision, normalized(t, s).Key().Digest, q.RoutingGeneration, "")
	if err != nil {
		t.Fatal(err)
	}
	back, err := observer(t, nil).Observe(changed, normalized(t, source(db)), true, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	still, _, _ := back.LookupRecovery(q)
	if !reflect.DeepEqual(saved, still) {
		t.Fatal("old source revived approval")
	}
	bad, _, err := back.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(back, id, clock()))
	rejected(t, bad, err)
	reload(t, back)
}

func TestRecoveryOrdinaryPreparationUsesNestedVersionTwo(t *testing.T) {
	r := recoveryRecord(t, observed(t, source(db), db))
	prepared, err := r.Prepare(clock(), preparations(t, source(db), db)...)
	if err != nil || prepared.data.delivery.SchemaVersion() != 2 || len(prepared.Snapshot().Recovery.Actions) != 0 {
		t.Fatal("ordinary preparation implicitly approved work", err)
	}
	id := prepared.Snapshot().Batches[0].Message.BatchID
	started, _, err := prepared.Begin(id, approvedContext(prepared, id, clock()))
	if err != nil {
		t.Fatal(err)
	}
	reload(t, started)
}
