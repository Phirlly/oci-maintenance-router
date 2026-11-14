package reconciliation

import (
	"bytes"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"testing"
	"time"
)

func TestRecoveryPausedClosureNeedsNoSendingDependencies(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	b := old.Snapshot().Batches[0]
	q := reviewedRecovery(t, old, noticeledger.CloseBatch, b.Message.BatchID)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	result, err := p.Run(leaseBudget(t), f.live.lease)
	if err != nil || !result.Snapshot().Completed {
		t.Fatal("paused closure failed", err)
	}
	actions := recoveryActions(t, f, n.Key().NoticeID)
	after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if len(actions) != 1 || actions[0].Status != noticeledger.RecoveryClosed || after.State != delivery.Closed || !bytes.Equal(after.Message.PayloadWithoutRoutingKey, b.Message.PayloadWithoutRoutingKey) || after.SecretVersion != b.SecretVersion || after.Ambiguous != b.Ambiguous {
		t.Fatal("closure lost immutable history", actions, after.State)
	}
	if f.listed.Load() != 0 || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("paused closure accessed sending dependencies")
	}
	before, _ := f.saved(t, n.Key().NoticeID).Encode()
	if _, err = p.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal("same closure did not return retained result", err)
	}
	again, _ := f.saved(t, n.Key().NoticeID).Encode()
	if !bytes.Equal(before, again) {
		t.Fatal("repeat closure rewrote the record")
	}
}

func TestRecoveryClosureDoesNotExpireActiveAttempt(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-2*time.Minute), delivery.Sending, false)
	q := reviewedRecovery(t, old, noticeledger.CloseRevision, n.Key().Digest)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	if _, err := p.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal(err)
	}
	actions := recoveryActions(t, f, n.Key().NoticeID)
	if len(actions) != 1 || actions[0].Status != noticeledger.RecoveryRejected || actions[0].Reason != "active_attempt" || f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Sending {
		t.Fatal("closure implicitly expired an active claim")
	}
}

func TestRecoveryNonLiveRetryIsDurablyRejectedWithoutSource(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	if _, err := p.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal(err)
	}
	actions := recoveryActions(t, f, n.Key().NoticeID)
	if len(actions) != 1 || actions[0].Status != noticeledger.RecoveryRejected || actions[0].Reason != "readiness_required" || f.listed.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("non-LIVE retry not explicitly rejected")
	}
}
