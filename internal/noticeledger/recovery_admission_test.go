package noticeledger

import (
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestRecoveryEntryPointsRejectMissingMigrationAndWrongActionKinds(t *testing.T) {
	r := retryReady(t, false)
	retry := retryRequest(t, r)
	close := closeRequest(t, r, CloseBatch, retry.TargetID)
	for _, old := range []Record{{}, observed(t, source(db), db)} {
		bad, a, ref, err := old.RetryRecovery(retry, retryContext(r, clock()))
		rejected(t, bad, err)
		if a.Status != "" || ref != (delivery.AttemptRef{}) {
			t.Fatal("legacy retry admitted")
		}
		bad, a, err = old.CloseRecovery(close, clock())
		rejected(t, bad, err)
		if a.Status != "" {
			t.Fatal("legacy closure admitted")
		}
		bad, a, err = observer(t, nil).ApproveRecovery(old, retry, normalized(t, source(db)), false, lookups(db), clock())
		rejected(t, bad, err)
		if a.Status != "" {
			t.Fatal("legacy approval admitted")
		}
		bad, _, err = old.BeginApproved("missing", retry.TargetID, retry.ComponentFingerprint, retryContext(r, clock()))
		rejected(t, bad, err)
	}
	bad, _, err := r.CloseRecovery(retry, clock())
	rejected(t, bad, err)
	bad, _, err = observer(t, nil).ApproveRecovery(r, retry, normalized(t, source(db)), false, lookups(db), clock())
	rejected(t, bad, err)
	c := retryContext(r, clock())
	c.RoutingGeneration = strings.Repeat("a", 64)
	bad, _, _, err = r.RetryRecovery(retry, c)
	rejected(t, bad, err)
}

func TestRecoveryCloseRetainsOnlyValidTargetAndReviewRejections(t *testing.T) {
	r := retryReady(t, false)
	for _, kind := range []ActionKind{CloseBatch, CloseRevision} {
		target := r.Snapshot().Revisions[0].Digest
		if kind == CloseBatch {
			target = r.Snapshot().Batches[0].Message.BatchID
		}
		q := closeRequest(t, r, kind, target)
		bad, _, err := r.CloseRecovery(q, time.Time{})
		rejected(t, bad, err)
		q.ReviewFingerprint = strings.Repeat("a", 64)
		next, a, err := r.CloseRecovery(q, clock())
		if err != nil || a.Status != RecoveryRejected || a.Reason != "stale_review" {
			t.Fatal("stale closure not retained", err)
		}
		reload(t, next)
		q.TargetID = strings.Repeat("b", 64)
		next, a, err = r.CloseRecovery(q, clock())
		if err != nil || a.Status != RecoveryRejected || a.Reason != "target_missing" {
			t.Fatal("missing target closure not retained", err)
		}
		reload(t, next)
	}
	claimed, _, ref, at := claimedRetry(t, false)
	q := closeRequest(t, claimed, CloseBatch, ref.BatchID)
	next, a, err := claimed.CloseRecovery(q, at)
	if err != nil || a.Status != RecoveryRejected || a.Reason != "active_attempt" {
		t.Fatal("active batch closed", err)
	}
	reload(t, next)
	base := recoveryRecord(t, observed(t, source(db)))
	q = closeRequest(t, base, CloseRevision, base.Snapshot().Revisions[0].Digest)
	closed, _, err := base.CloseRecovery(q, clock())
	if err != nil {
		t.Fatal(err)
	}
	q = closeRequest(t, closed, CloseRevision, q.TargetID)
	q.RequestID = "second-close"
	next, a, err = closed.CloseRecovery(q, clock())
	if err != nil || a.Reason != "target_terminal" {
		t.Fatal("closed revision reopened", err)
	}
	reload(t, next)
}

func TestRecoveryCapacityFailureReturnsNoApprovalClosureOrRejection(t *testing.T) {
	r := fullSourceRecord(t, recoveryRecord(t, observed(t, source(db), db)))
	q := approvalRequest(t, r, r.Snapshot().Revisions[0].Digest)
	bad, a, err := observer(t, nil).ApproveRecovery(r, q, normalized(t, source(db)), false, lookups(db), clock(), preparations(t, source(db), db)...)
	rejected(t, bad, err)
	if a.Status != "" {
		t.Fatal("failed approval consumed ID")
	}
	close := closeRequest(t, r, CloseRevision, q.TargetID)
	bad, a, err = r.CloseRecovery(close, clock())
	rejected(t, bad, err)
	if a.Status != "" {
		t.Fatal("failed closure consumed ID")
	}
	bad, a, err = r.RejectRecovery(q, "readiness_required", clock())
	rejected(t, bad, err)
	if a.Status != "" {
		t.Fatal("failed rejection consumed ID")
	}
}
