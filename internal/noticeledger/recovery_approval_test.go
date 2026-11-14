package noticeledger

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
)

func TestRecoveryApprovalKeepsHoldsAndRequiresItsOwnBegin(t *testing.T) {
	for _, held := range []bool{false, true} {
		r, q, a := approvedRecord(t, held)
		if !r.Follows(Record{data: r.parent}) || len(a.BatchIDs) != 1 || a.CompletedAt == nil || *a.CompletedAt != a.RecordedAt {
			t.Fatal("approval lacks atomic work association")
		}
		if (r.Snapshot().HoldReason != "") != held || r.Snapshot().Batches[0].State != delivery.Prepared {
			t.Fatal("historical hold cleared or approved work held")
		}
		reload(t, r)
		for _, current := range []Record{r, mustDecodeRecovery(t, r)} {
			id := a.BatchIDs[0]
			c := approvedContext(current, id, clock())
			bad, _, err := current.Begin(id, c)
			rejected(t, bad, err)
			started, ref, err := current.BeginApproved(q.RequestID, id, q.ComponentFingerprint, c)
			if err != nil || ref.BatchID != id || started.Snapshot().Batches[0].AttemptKind != delivery.AutomaticAttempt {
				t.Fatal("approved attempt blocked", err)
			}
			reload(t, started)
			for _, component := range []string{"", strings.Repeat("c", 64)} {
				bad, _, err := current.BeginApproved(q.RequestID, id, component, c)
				rejected(t, bad, err)
			}
			repeat, saved, err := (*Observer)(nil).ApproveRecovery(current, q, notices.Notice{}, false, nil, time.Time{})
			if err != nil || repeat.Follows(current) || !bytes.Equal(encoded(t, repeat), encoded(t, current)) || !reflect.DeepEqual(saved, a) {
				t.Fatal("approval replay changed work", err)
			}
		}
	}
}

func TestRecoveryApprovalOnlyIncludesNewExplicitWork(t *testing.T) {
	n := normalized(t, source(db, otherDB))
	found := lookups(db)
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
	first, a, err := observer(t, nil).ApproveRecovery(r, q, n, false, found, clock(), approvalDrafts(t, n, found, db)...)
	if err != nil || len(a.BatchIDs) != 1 {
		t.Fatal(err)
	}
	later, err := observer(t, nil).Observe(first, n, false, lookups(db, otherDB), clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(later.Snapshot().Batches) != 1 || later.Snapshot().Revisions[0].Resources[1].BatchID != "" {
		t.Fatal("approval silently expanded")
	}
	bad, err := later.Prepare(clock().Add(time.Second), approvalDrafts(t, n, lookups(db, otherDB), otherDB)...)
	rejected(t, bad, err)
	q2 := approvalRequest(t, later, n.Key().Digest)
	q2.RequestID = "approve-2"
	second, a2, err := observer(t, nil).ApproveRecovery(later, q2, n, false, lookups(db, otherDB), clock().Add(time.Second), approvalDrafts(t, n, lookups(db, otherDB), otherDB)...)
	if err != nil || len(second.Snapshot().Batches) != 2 || len(a2.BatchIDs) != 1 || a.BatchIDs[0] == a2.BatchIDs[0] {
		t.Fatal("separate explicit work failed", err)
	}
	old, _, err := second.LookupRecovery(q)
	if err != nil || !reflect.DeepEqual(old, a) {
		t.Fatal("approval association changed")
	}
	reload(t, second)
}
