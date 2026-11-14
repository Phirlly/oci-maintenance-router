package noticeledger

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func closeRequest(t *testing.T, r Record, kind ActionKind, target string) RecoveryRequest {
	t.Helper()
	q := RecoveryRequest{RequestID: "close-1", Kind: kind, TargetID: target, RoutingGeneration: r.Snapshot().Revisions[0].RoutingGeneration}
	var err error
	q.ReviewFingerprint, err = r.ReviewFingerprint(kind, target, q.RoutingGeneration, "")
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestRecoveryCloseBatchPreservesAttemptHistoryAndRequestResult(t *testing.T) {
	r, oldRequest, ref, at := claimedRetry(t, true)
	r, err := r.StopUnsent(ref, at)
	if err != nil {
		t.Fatal(err)
	}
	oldAction, _, _ := r.LookupRecovery(oldRequest)
	before := r.Snapshot().Batches[0]
	q := closeRequest(t, r, CloseBatch, ref.BatchID)
	closed, a, err := r.CloseRecovery(q, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	after := closed.Snapshot().Batches[0]
	if !closed.Follows(r) || a.Status != RecoveryClosed || after.State != delivery.Closed || !after.Ambiguous || after.AttemptCount != before.AttemptCount || !reflect.DeepEqual(after.Message, before.Message) || after.SecretVersion != before.SecretVersion || !after.FirstAttemptAt.Equal(before.FirstAttemptAt) {
		t.Fatal("closure rewrote saved history")
	}
	row := closed.Snapshot().Revisions[0].Resources[0]
	if row.Disposition != "CLOSED" || row.Reason != "operator" || row.BatchID != ref.BatchID || row.DestinationID != "payments" {
		t.Fatal("closure lost assignment")
	}
	got, _, _ := closed.LookupRecovery(oldRequest)
	if !reflect.DeepEqual(got, oldAction) {
		t.Fatal("closure rewrote retry result")
	}
	reload(t, closed)
	repeat, saved, err := closed.CloseRecovery(q, time.Time{})
	if err != nil || repeat.Follows(closed) || !bytes.Equal(encoded(t, repeat), encoded(t, closed)) || !reflect.DeepEqual(a, saved) {
		t.Fatal("closure request repeated", err)
	}
	q2 := closeRequest(t, closed, CloseBatch, ref.BatchID)
	q2.RequestID = "close-2"
	next, a, err := closed.CloseRecovery(q2, at.Add(time.Second))
	if err != nil || a.Status != RecoveryRejected || a.Reason != "target_terminal" {
		t.Fatal("terminal closure repeated", err)
	}
	reload(t, next)
}

func TestRecoveryCloseRevisionPreservesRowsAcrossObservation(t *testing.T) {
	for _, empty := range []bool{false, true} {
		s := source(db, otherDB)
		if empty {
			s.Resources = nil
		}
		r := recoveryRecord(t, observed(t, s))
		q := closeRequest(t, r, CloseRevision, r.Snapshot().Revisions[0].Digest)
		closed, a, err := r.CloseRecovery(q, clock())
		if err != nil || a.Status != RecoveryClosed {
			t.Fatal(err)
		}
		for _, row := range closed.Snapshot().Revisions[0].Resources {
			if row.Disposition != "CLOSED" || row.Reason != "operator" || row.BatchID != "" || row.DestinationID != "" {
				t.Fatal("unassigned row not closed")
			}
		}
		observed, err := observer(t, nil).Observe(closed, normalized(t, s), false, lookups(db, otherDB), clock().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(observed.Snapshot().Revisions[0].Resources, closed.Snapshot().Revisions[0].Resources) {
			t.Fatal("closed rows reopened")
		}
		reload(t, observed)
		if !empty {
			bad, err := observed.Prepare(clock().Add(time.Second), preparations(t, s, db, otherDB)...)
			rejected(t, bad, err)
		}
		s.Reference = text("genuine new revision")
		newer, err := observer(t, nil).Observe(observed, normalized(t, s), true, lookups(db, otherDB), clock().Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if len(newer.Snapshot().Revisions) != 2 || recoveryRevisionClosed(newer.data, normalized(t, s).Key().Digest) {
			t.Fatal("closure expanded to new revision")
		}
		reload(t, newer)
	}
}

func TestRecoveryCloseRefusesSendingAndPreservesAcceptedPeer(t *testing.T) {
	n := normalized(t, source(db, otherDB))
	found := lookups(db, otherDB)
	lookup := found[otherDB]
	lookup.Metadata.FreeformTags = map[string]string{"owner": "video"}
	found[otherDB] = lookup
	r, err := observer(t, nil).Observe(Record{}, n, false, found, clock())
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Prepare(clock(), approvalDrafts(t, n, found, db, otherDB)...)
	if err != nil {
		t.Fatal(err)
	}
	r = recoveryRecord(t, r)
	id := r.Snapshot().Batches[0].Message.BatchID
	r, ref, err := r.Begin(id, approvedContext(r, id, clock()))
	if err != nil {
		t.Fatal(err)
	}
	q := closeRequest(t, r, CloseRevision, n.Key().Digest)
	rejectedRecord, a, err := r.CloseRecovery(q, clock())
	if err != nil || a.Status != RecoveryRejected || a.Reason != "active_attempt" || !reflect.DeepEqual(r.Snapshot().Batches, rejectedRecord.Snapshot().Batches) {
		t.Fatal("active closure partially changed state", err)
	}
	r, err = rejectedRecord.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q = closeRequest(t, r, CloseRevision, n.Key().Digest)
	q.RequestID = "close-2"
	closed, a, err := r.CloseRecovery(q, clock())
	if err != nil || a.Status != RecoveryClosed {
		t.Fatal(err)
	}
	for _, b := range closed.Snapshot().Batches {
		want := delivery.Closed
		if b.Message.BatchID == id {
			want = delivery.APIAccepted
		}
		if b.State != want {
			t.Fatal("peer receipt lost")
		}
	}
	reload(t, closed)
}
