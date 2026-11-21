package noticeledger

import (
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestAnnouncementSavedMessagesKeepAssignmentsThroughOutcomes(t *testing.T) {
	for _, name := range []string{"announcement-mysql", "announcement-compute", "announcement-mixed"} {
		r := announcementRecord(t, name)
		message := r.Snapshot().Batches[0].Message
		sending, ref := begin(t, r, message.BatchID, clock())
		accepted, err := reload(t, sending).Finish(ref, 202, clock(), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range accepted.Snapshot().Revisions[0].Resources {
			if row.Disposition != "API_ACCEPTED" || row.BatchID != message.BatchID {
				t.Fatal("receipt lost assignment")
			}
		}
		expired, err := sending.Recover(clock().Add(time.Minute))
		if err != nil || expired.Snapshot().Batches[0].State != delivery.RetryWait {
			t.Fatal(err)
		}
		held, err := expired.Hold(delivery.ReasonOperator, clock().Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		q := retryRequest(t, held)
		at := clock().Add(48 * time.Hour)
		manual, a, manualRef, err := held.RetryRecovery(q, retryContext(held, at))
		if err != nil || a.Status != RecoveryConsumed {
			t.Fatal("manual attempt blocked", err)
		}
		finished, err := reload(t, manual).Finish(manualRef, 503, at, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		a, exists, err := finished.LookupRecovery(q)
		if err != nil || !exists || a.Status != RecoveryReviewed || a.Reason != "transient" {
			t.Fatal("consumed history not completed", err)
		}
		close := closeRequest(t, finished, CloseBatch, message.BatchID)
		if reason, err := finished.ClosureBlocker(close.Kind, close.TargetID); err != nil || reason != "" {
			t.Fatal(reason, err)
		}
		closed, action, err := finished.CloseRecovery(close, at)
		if err != nil || action.Status != RecoveryClosed {
			t.Fatal(err)
		}
		for _, row := range closed.Snapshot().Revisions[0].Resources {
			if row.Disposition != "CLOSED" || row.BatchID != message.BatchID {
				t.Fatal("closure lost assignment")
			}
		}
		for _, next := range []Record{accepted, expired, manual, finished, closed} {
			next = reload(t, next)
			if !reflect.DeepEqual(message, next.Snapshot().Batches[0].Message) || next.Snapshot().Batches[0].SecretVersion != 7 {
				t.Fatal("outcome changed saved request")
			}
		}
		view := closed.Snapshot()
		view.Revisions[0].Resources[0].ResourceID = "changed"
		view.Recovery.Actions[0].Request.RequestID = "changed"
		if reflect.DeepEqual(view, closed.Snapshot()) {
			t.Fatal("snapshot aliases state")
		}
	}
}

func TestAnnouncementApprovalInvalidationRemainsStickyAndKeepsReceipt(t *testing.T) {
	for _, cause := range []string{"source", "configuration"} {
		for _, accepted := range []bool{false, true} {
			r, q, a := approvedRecord(t, true)
			r = migrateAnnouncements(t, r)
			at := clock().Add(time.Second)
			id := a.BatchIDs[0]
			started, ref, err := r.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(r, id, at))
			if err != nil {
				t.Fatal(err)
			}
			var changed Record
			if cause == "source" {
				changed, err = started.RecordSourceUnavailable(404, at)
			} else {
				changed, err = observer(t, func(v map[string]any) { v["pagerDutyRegion"] = "EU" }).Observe(started, normalized(t, source(db)), false, lookups(db), at)
			}
			if err != nil {
				t.Fatal(err)
			}
			reverted, err := observer(t, nil).Observe(changed, normalized(t, source(db)), false, lookups(db), at)
			if err != nil {
				t.Fatal(err)
			}
			a, _, err = reverted.LookupRecovery(q)
			if err != nil || a.InvalidatedAt == nil {
				t.Fatal("approval revived", err)
			}
			status, retry := 503, at
			want := delivery.ReviewRequired
			if accepted {
				status, retry, want = 202, time.Time{}, delivery.APIAccepted
			}
			next, err := reverted.Finish(ref, status, at, retry)
			if err != nil || next.Snapshot().Batches[0].State != want {
				t.Fatal("outcome lost or retries revived", err)
			}
			bad, _, err := next.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(next, id, at))
			rejected(t, bad, err)
			reload(t, next)
		}
	}
}

func TestAnnouncementObservationPreservesClosedAndAssignedRows(t *testing.T) {
	n := source(db, otherDB)
	r := migrateAnnouncements(t, recoveryRecord(t, observed(t, n, db, otherDB)))
	at := clock().Add(time.Second)
	r, err := r.Prepare(at, preparations(t, n, db)...)
	if err != nil {
		t.Fatal(err)
	}
	q := closeRequest(t, r, CloseRevision, r.Snapshot().Revisions[0].Digest)
	r, a, err := r.CloseRecovery(q, at)
	if err != nil || a.Status != RecoveryClosed {
		t.Fatal(err)
	}
	before := r.Snapshot().Revisions[0].Resources
	next, err := observer(t, nil).Observe(r, normalized(t, n), false, lookups(db, otherDB), at)
	if err != nil || next.Snapshot().SchemaVersion != 4 || !reflect.DeepEqual(before, next.Snapshot().Revisions[0].Resources) {
		t.Fatal("observation reopened work", err)
	}
	reload(t, next)
}
