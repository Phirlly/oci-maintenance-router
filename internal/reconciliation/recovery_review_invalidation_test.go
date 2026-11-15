package reconciliation

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

func TestRecoveryReviewNeverInventsApprovalInvalidationTime(t *testing.T) {
	raw := processingNotice(t, 1, processingDB).Snapshot()
	raw.ChainID = chain("group")
	n := normalizeCandidate(t, raw)
	raw = processingNotice(t, 2, processingDB).Snapshot()
	raw.ChainID = chain("group")
	peer := normalizeCandidate(t, raw)
	f := processingFixtureFor(t, n, peer)
	record := seedApprovedRecovery(t, f, n, true)
	// A terminal batch stays byte-identical when the changed component invalidates
	// its earlier approval. This isolates invalidation time from batch transitions.
	at := time.Now().Add(-time.Second)
	b := record.Snapshot().Batches[0]
	q := record.Snapshot().Recovery.Actions[0].Request
	record, ref, err := record.BeginApproved(q.RequestID, b.Message.BatchID, q.ComponentFingerprint, delivery.AttemptContext{OwnerToken: "approved-worker", RoutingGeneration: b.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err == nil {
		record, err = record.Finish(ref, 202, at, time.Time{})
	}
	if err != nil {
		t.Fatal(err)
	}
	before, _ := record.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), before)
	r := reviewFixture(t, f)
	view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if err != nil || view.Blocker != "saved_state_refresh_required" || view.Fingerprint != "" || !bytes.Equal(before, after) || view.Projected.Snapshot().Recovery.Actions[0].InvalidatedAt == nil {
		t.Fatal("invented invalidation or changed stored record", view.Blocker, err)
	}
	if _, err = f.p.discovery.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal(err)
	}
	recorded := f.saved(t, n.Key().NoticeID).Snapshot().Recovery.Actions[0].InvalidatedAt
	if recorded == nil {
		t.Fatal("ordinary discovery did not save invalidation")
	}
	view, err = r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	if err != nil || view.Blocker == "saved_state_refresh_required" || view.Projected.Snapshot().Recovery.Actions[0].InvalidatedAt == nil || *view.Projected.Snapshot().Recovery.Actions[0].InvalidatedAt != *recorded {
		t.Fatal("recorded invalidation was restamped", view.Blocker, err)
	}
}
