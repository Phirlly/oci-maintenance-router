package noticeledger

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryHistoryRejectsDuplicateMissingOrRegressedAttempts(t *testing.T) {
	r, q, ref, at := claimedRetry(t, false)
	r, err := r.Finish(ref, 0, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a-earlier-sort", "z-later-sort"} {
		q = retryRequest(t, r)
		q.RequestID = id
		started, _, nextRef, err := r.RetryRecovery(q, retryContext(r, at))
		if err != nil {
			t.Fatal(err)
		}
		r, err = started.StopUnsent(nextRef, at)
		if err != nil {
			t.Fatal(err)
		}
	}
	reload(t, r)
	rejectRecoveryWire(t, r, map[string]func(*wireRecord){
		"missing_middle": func(w *wireRecord) { w.Recovery.Actions = w.Recovery.Actions[1:] },
		"duplicate_attempt": func(w *wireRecord) {
			a := copyRecoveryAction(w.Recovery.Actions[0])
			a.Request.RequestID = "b-duplicate"
			a.IntentDigest = recoveryIntentDigest(w.recordData, a.Request)
			w.Recovery.Actions = append(w.Recovery.Actions, a)
			sortRecovery(w.Recovery)
		},
		"forgotten_ambiguity": func(w *wireRecord) { w.Recovery.Actions[0].Ambiguous = false },
		"invented_success": func(w *wireRecord) {
			w.Recovery.Actions[0].Status = RecoveryAccepted
			w.Recovery.Actions[0].Reason = ""
		},
		"invalid_review_reason": func(w *wireRecord) { w.Recovery.Actions[0].Reason = "retry_horizon" },
		"wrong_status":          func(w *wireRecord) { w.Recovery.Actions[0].Status = "unknown" },
	})
}

func TestRecoveryResultsOwnReferencesAndConcurrentProposalsStayIndependent(t *testing.T) {
	r, q, _, _ := claimedRetry(t, false)
	before := encoded(t, r)
	a, _, err := r.LookupRecovery(q)
	if err != nil {
		t.Fatal(err)
	}
	a.Attempt.OwnerToken = "modified"
	a.Attempt.ExpiresAt = "modified"
	view := r.Snapshot()
	view.Recovery.Actions[0].Attempt.AttemptCount = 99
	if !bytes.Equal(before, encoded(t, r)) {
		t.Fatal("returned attempt aliases retained state")
	}
	approved, aq, action := approvedRecord(t, true)
	before = encoded(t, approved)
	action.BatchIDs[0] = "modified"
	view = approved.Snapshot()
	view.Recovery.Actions[0].BatchIDs[0] = "modified"
	if !bytes.Equal(before, encoded(t, approved)) {
		t.Fatal("approval members alias retained state")
	}
	held, err := approved.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	before = encoded(t, held)
	a, _, err = held.LookupRecovery(aq)
	if err != nil {
		t.Fatal(err)
	}
	*a.InvalidatedAt = "modified"
	if !bytes.Equal(before, encoded(t, held)) {
		t.Fatal("invalidation time aliases retained state")
	}
	base := retryReady(t, false)
	before = encoded(t, base)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := retryRequest(t, base)
			req.RequestID = fmt.Sprintf("parallel-%d", i)
			c := retryContext(base, clock())
			c.OwnerToken = req.RequestID
			next, a, ref, err := base.RetryRecovery(req, c)
			if err != nil || !next.Follows(base) || len(next.Snapshot().Recovery.Actions) != 1 || ref.OwnerToken != req.RequestID || a.Request.RequestID != req.RequestID {
				t.Error("parallel proposal crossed request boundaries", err)
			}
		}(i)
	}
	wg.Wait()
	if !bytes.Equal(before, encoded(t, base)) {
		t.Fatal("concurrent proposals mutated base")
	}
}

func TestRecoveryApprovalRejectsOverlapAndDuplicateHistoricalAssociation(t *testing.T) {
	n := normalized(t, source(db, otherDB))
	found := lookups(db, otherDB)
	r, err := observer(t, nil).Observe(Record{}, n, false, found, clock())
	if err != nil {
		t.Fatal(err)
	}
	r = recoveryRecord(t, r)
	q := approvalRequest(t, r, n.Key().Digest)
	r, a, err := observer(t, nil).ApproveRecovery(r, q, n, false, found, clock(), approvalDrafts(t, n, found, db)...)
	if err != nil {
		t.Fatal(err)
	}
	before := r.Snapshot()
	nextQ := approvalRequest(t, r, n.Key().Digest)
	nextQ.RequestID = "approve-2"
	bad, _, err := observer(t, nil).ApproveRecovery(r, nextQ, n, false, found, clock(), approvalDrafts(t, n, found, db, otherDB)...)
	rejected(t, bad, err)
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("overlap partially assigned peer")
	}
	rejectRecoveryWire(t, r, map[string]func(*wireRecord){"duplicate_association": func(w *wireRecord) {
		duplicate := copyRecoveryAction(a)
		duplicate.Request.RequestID = "approve-2"
		duplicate.IntentDigest = recoveryIntentDigest(w.recordData, duplicate.Request)
		w.Recovery.Actions = append(w.Recovery.Actions, duplicate)
	}})
	missing := normalized(t, source(otherDB))
	request := q
	request.RequestID = "missing-approval"
	request.TargetID = missing.Key().Digest
	request.ReviewFingerprint = strings.Repeat("a", 64)
	next, result, err := observer(t, nil).ApproveRecovery(r, request, missing, false, lookups(otherDB), clock())
	if err != nil || result.Reason != "target_missing" {
		t.Fatal("unknown target approval not rejected", err)
	}
	reload(t, next)
}
