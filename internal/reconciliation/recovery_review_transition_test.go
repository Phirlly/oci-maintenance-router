package reconciliation

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
)

func TestRecoveryReviewRequiresSavedTimestampTransitionsFirst(t *testing.T) {
	for _, state := range []delivery.State{delivery.Prepared, delivery.RetryWait} {
		t.Run(string(state), func(t *testing.T) {
			raw := processingNotice(t, 1, processingDB).Snapshot()
			raw.ChainID = chain("new-peer")
			n := normalizeCandidate(t, raw)
			peer := processingNotice(t, 2, processingDB).Snapshot()
			peer.ChainID = chain("new-peer")
			f := processingFixtureWithBaseline(t, []notices.Notice{n}, n, normalizeCandidate(t, peer))
			old := f.seedBatch(t, n, time.Now().Add(-time.Minute), state, false)
			before, _ := old.Encode()
			r := reviewFixture(t, f)
			view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
			after, _ := f.saved(t, n.Key().NoticeID).Encode()
			if err != nil || view.Blocker != "saved_state_refresh_required" || view.Fingerprint != "" || !bytes.Equal(before, after) {
				t.Fatal("invented durable transition", view.Blocker, err)
			}
		})
	}
}
