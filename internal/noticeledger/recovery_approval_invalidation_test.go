package noticeledger

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryApprovalInvalidationSurvivesSourceRecoveryAndSending(t *testing.T) {
	for _, sending := range []bool{false, true} {
		for _, accepted := range []bool{false, true} {
			r, q, a := approvedRecord(t, true)
			id := a.BatchIDs[0]
			var ref delivery.AttemptRef
			var err error
			if sending {
				r, ref, err = r.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(r, id, clock()))
				if err != nil {
					t.Fatal(err)
				}
			}
			r, err = r.RecordSourceUnavailable(404, clock())
			if err != nil {
				t.Fatal(err)
			}
			r, err = observer(t, nil).Observe(r, normalized(t, source(db)), false, lookups(db), clock())
			if err != nil {
				t.Fatal(err)
			}
			a, _, err = r.LookupRecovery(q)
			if err != nil || a.InvalidatedAt == nil || a.InvalidationReason != "source_unavailable" {
				t.Fatal("approval revived", err)
			}
			if sending {
				status := 503
				retry := clock()
				if accepted {
					status = 202
					retry = time.Time{}
				}
				r, err = r.Finish(ref, status, clock(), retry)
				if err != nil {
					t.Fatal(err)
				}
				want := delivery.ReviewRequired
				if accepted {
					want = delivery.APIAccepted
				}
				if r.Snapshot().Batches[0].State != want {
					t.Fatal("invalidated attempt regained automatic retries")
				}
			}
			bad, _, err := r.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(r, id, clock()))
			rejected(t, bad, err)
			reload(t, r)
		}
	}
}

func TestRecoveryApprovalRoutineObservationKeepsApprovalAndExplicitHoldInvalidates(t *testing.T) {
	r, q, a := approvedRecord(t, true)
	id := a.BatchIDs[0]
	observed, err := observer(t, nil).Observe(r, normalized(t, source(db)), false, lookups(db), clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := observed.LookupRecovery(q)
	if err != nil || got.InvalidatedAt != nil || observed.Snapshot().Batches[0].State != delivery.Prepared {
		t.Fatal("routine observation invalidated approval", err)
	}
	held, err := observed.Hold(delivery.ReasonOperator, clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err = held.LookupRecovery(q)
	if err != nil || got.InvalidatedAt == nil || held.Snapshot().Batches[0].State != delivery.ReviewRequired {
		t.Fatal("explicit hold did not invalidate")
	}
	bad, _, err := held.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(held, id, clock().Add(time.Second)))
	rejected(t, bad, err)
	reload(t, held)
}

func TestRecoveryApprovalInvalidationStillHoldsAfterConfigurationReverts(t *testing.T) {
	r, q, a := approvedRecord(t, false)
	id := a.BatchIDs[0]
	started, ref, err := r.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(r, id, clock()))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := observer(t, func(v map[string]any) { v["pagerDutyRegion"] = "EU" }).Observe(started, normalized(t, source(db)), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := observer(t, nil).Observe(changed, normalized(t, source(db)), false, lookups(db), clock())
	if err != nil {
		t.Fatal(err)
	}
	next, err := reverted.Finish(ref, 503, clock(), clock())
	if err != nil {
		t.Fatal(err)
	}
	batch := next.Snapshot().Batches[0]
	if batch.State != delivery.ReviewRequired || batch.Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("invalidated work remained automatically retryable after configuration reverted", batch.State, batch.Reason)
	}
	bad, _, err := next.BeginApproved(q.RequestID, id, q.ComponentFingerprint, approvedContext(next, id, clock()))
	rejected(t, bad, err)
	manual := retryRequest(t, next)
	manual.RequestID = "retry-invalidated"
	retried, action, _, err := next.RetryRecovery(manual, retryContext(next, clock()))
	if err != nil || action.Status != RecoveryConsumed {
		t.Fatal("explicit recovery stranded", err)
	}
	reload(t, retried)
}
