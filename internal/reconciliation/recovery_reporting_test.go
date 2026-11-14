package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"testing"
)

func TestRecoveryRetainedApprovalReportsInvalidationFromCurrentDiscovery(t *testing.T) {
	v := processingNotice(t, 1, processingDB).Snapshot()
	v.ChainID = chain("group")
	n := normalizeCandidate(t, v)
	v = processingNotice(t, 2, processingDB).Snapshot()
	v.ChainID = chain("group")
	peer := normalizeCandidate(t, v)
	f := processingFixtureFor(t, n, peer)
	old := seedApprovedRecovery(t, f, n, true)
	q := old.Snapshot().Recovery.Actions[0].Request
	f.configure(t, func(v map[string]any) {
		v["schemaVersion"] = 7
		v["recovery"] = map[string]any{"requestId": q.RequestID, "noticeId": n.Key().NoticeID, "kind": string(q.Kind), "targetId": q.TargetID, "reviewFingerprint": q.ReviewFingerprint, "routingGeneration": q.RoutingGeneration, "componentFingerprint": q.ComponentFingerprint}
	})
	s := f.run(t)
	a := recoveryActions(t, f, n.Key().NoticeID)[0]
	if a.InvalidatedAt == nil || a.InvalidationReason != "source_changed" || s.Recovery == nil || s.Recovery.InvalidationReason != a.InvalidationReason || f.sent.Load() != 0 {
		t.Fatal("result concealed approval invalidation saved by this pass", s.Recovery, a.InvalidationReason)
	}
}

func TestRecoveryInvalidatedUnheldApprovalRemainsVisible(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := seedApprovedRecovery(t, f, n, false)
	f.configure(t, func(v map[string]any) {
		v["routes"] = append(v["routes"].([]any), map[string]any{"ownerValue": "extra", "destinationId": "payments"})
	})
	s := f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if after.HoldReason != "" || after.Recovery.Actions[0].InvalidatedAt == nil || after.Batches[0].State != delivery.ReviewRequired {
		t.Fatal("fixture did not isolate batch-only invalidation")
	}
	found := false
	for _, issue := range s.Issues {
		if issue.BatchID == old.Snapshot().Batches[0].Message.BatchID && issue.Code == "configuration_changed" {
			found = true
		}
	}
	if !found || f.sent.Load() != 0 || f.exact.Load() != 0 {
		t.Fatal("blocked approval was silently hidden", s)
	}
}
