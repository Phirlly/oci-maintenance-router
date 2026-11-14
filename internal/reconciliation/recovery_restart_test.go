package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
	"time"
)

func TestRecoveryConsumedManualClaimNeverResumes(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if processingWriteState(t, f, c) == delivery.Sending {
			cancel()
		}
		return nil
	}
	_, err := f.p.Run(ctx, f.live.lease)
	if !errors.Is(err, context.Canceled) || f.sent.Load() != 0 {
		t.Fatal("canceled claim dispatched", err)
	}
	f.live.objects.after = nil
	claimed := f.saved(t, n.Key().NoticeID).Snapshot()
	if claimed.Recovery.Actions[0].Status != noticeledger.RecoveryConsumed || claimed.Batches[0].State != delivery.Sending {
		t.Fatal("consumption and claim were not atomic")
	}
	f.run(t)
	if f.sent.Load() != 0 || f.exact.Load() != 1 {
		t.Fatal("active consumed request resumed")
	}

}

func TestRecoveryExpiredConsumedManualClaimBecomesReviewWithoutSending(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	at := time.Now().Add(-2 * time.Minute)
	old := f.seedBatch(t, n, at, delivery.Prepared, true)
	projected, err := old.MigrateRecovery(at)
	if err != nil {
		t.Fatal(err)
	}
	q := reviewedRecovery(t, projected, noticeledger.RetryBatch, projected.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	intent := (&processingRun{owner: f.p}).recoveryRequest()
	claimed, action, _, err := projected.RetryRecovery(intent, delivery.AttemptContext{OwnerToken: "previous-worker", RoutingGeneration: old.Snapshot().Batches[0].Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil || action.Status != noticeledger.RecoveryConsumed {
		t.Fatal("invalid expired fixture", err)
	}
	body, _ := claimed.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err != nil || !r.Snapshot().Completed || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Status != string(noticeledger.RecoveryReviewed) || f.sent.Load() != 0 || f.exact.Load() != 0 {
		t.Fatal("expired consumed request retried or remained active", err, r.Snapshot())
	}
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if b.State != delivery.ReviewRequired || !b.Ambiguous || b.AttemptCount != claimed.Snapshot().Batches[0].AttemptCount {
		t.Fatal("expiry discarded uncertainty or consumed another attempt")
	}
}

func TestRecoveryIssuedAcceptanceSurvivesCallerCancellation(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	send := f.p.senders["US"]
	f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
		r, err := send(ctx, body, key)
		cancel()
		return r, err
	}
	r, err := f.p.Run(ctx, f.live.lease)
	if !errors.Is(err, context.Canceled) || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Status != string(noticeledger.RecoveryAccepted) || r.Snapshot().Accepted != 1 || !r.Snapshot().Attempts[0].Confirmed {
		t.Fatal("observed acceptance not persisted after cancellation", err, r.Snapshot())
	}
	if recoveryActions(t, f, n.Key().NoticeID)[0].Status != noticeledger.RecoveryAccepted {
		t.Fatal("manual receipt history lost")
	}
}
