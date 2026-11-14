package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"testing"
	"time"
)

func TestMaintenanceRecoversExpiredSendWithoutResettingAmbiguity(t *testing.T) {
	for _, held := range []bool{false, true} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-2*time.Minute), delivery.Sending, held)
		s := f.run(t)
		b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
		if !b.Ambiguous || !b.FirstAttemptAt.Equal(old.Snapshot().Batches[0].FirstAttemptAt) {
			t.Fatal("recovery erased original uncertainty/time")
		}
		if held {
			if s.Accepted != 0 || f.sent.Load() != 0 || b.State != delivery.ReviewRequired {
				t.Fatal("held expired claim not reviewed")
			}
		} else if s.Accepted != 1 || b.State != delivery.APIAccepted || b.AttemptCount != 2 {
			t.Fatal("expired claim did not recover with exact identity")
		}
	}
}

func TestMaintenanceAutomaticHorizonRequiresReviewOnRestart(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.seedBatch(t, n, time.Now().Add(-25*time.Hour), delivery.RetryWait, false)
	s := f.run(t)
	b := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if s.Accepted != 0 || f.sent.Load() != 0 || f.exact.Load() != 0 || b.State != delivery.ReviewRequired || b.Reason != delivery.ReasonHorizon {
		t.Fatal("expired automatic horizon retried")
	}
}
