package reconciliation

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestRecoveryReviewDoesNotOfferManualRetryForUnblockedOrAcceptedBatch(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.APIAccepted} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		record := f.seedBatch(t, n, time.Now().Add(-time.Minute), state, false)
		r := reviewFixture(t, f)
		view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.RetryBatch, record.Snapshot().Batches[0].Message.BatchID)
		if err != nil || view.Blocker == "" || view.Fingerprint != "" || f.sent.Load() != 0 {
			t.Fatal("nonretryable batch offered recovery", state, view.Blocker, err)
		}
	}
}

func TestRecoveryReviewDoesNotApproveNoticesWithoutUsableMaintenanceScope(t *testing.T) {
	for _, kind := range []string{"OUTAGE", "SCHEDULED_MAINTENANCE"} {
		raw := processingNotice(t, 1).Snapshot()
		raw.AnnouncementType = kind
		n := normalizeCandidate(t, raw)
		f := processingFixtureFor(t, n)
		seedReviewRevision(t, f, n)
		r := reviewFixture(t, f)
		view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
		if err != nil || view.Blocker != "no_eligible_work" || view.Fingerprint != "" || f.sent.Load() != 0 {
			t.Fatal("unsupported regional notice approved", kind, view.Blocker, err)
		}
	}
}
