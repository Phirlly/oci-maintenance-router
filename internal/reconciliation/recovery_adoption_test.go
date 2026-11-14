package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"testing"
)

func TestRecoveryDiscoveryPreservesApprovalUnlessComponentChanges(t *testing.T) {
	for _, changed := range []bool{false, true} {
		v := processingNotice(t, 1, processingDB).Snapshot()
		v.ChainID = chain("maintenance-group")
		n := normalizeCandidate(t, v)
		peer := processingNotice(t, 2, processingDB).Snapshot()
		peer.ChainID = chain("maintenance-group")
		current := []notices.Notice{n}
		if changed {
			current = append(current, normalizeCandidate(t, peer))
		}
		f := processingFixtureFor(t, current...)
		seedApprovedRecovery(t, f, n, true)
		if _, err := f.p.discovery.Run(leaseBudget(t), f.live.lease); err != nil {
			t.Fatal("discovery rejected action-aware state", err)
		}
		a := recoveryActions(t, f, n.Key().NoticeID)[0]
		if (a.InvalidatedAt != nil) != changed {
			t.Fatal("wrong approval invalidation", changed, a)
		}
		if changed && (a.InvalidationReason != "source_changed" || f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.ReviewRequired || f.saved(t, noticeID(2)).Snapshot().SchemaVersion != 2) {
			t.Fatal("component change was not held or ordinary peer was migrated")
		}
	}
}

func TestRecoveryApprovedBatchContinuesDespiteHoldAndBaseline(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	old := seedApprovedRecovery(t, f, n, true)
	s := f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if s.Accepted != 1 || s.Prepared != 0 || f.current.Load() != 0 || f.exact.Load() != 1 || f.sent.Load() != 1 || after.HoldReason != old.Snapshot().HoldReason || after.Batches[0].State != delivery.APIAccepted {
		t.Fatal("approved immutable baseline work did not continue", s)
	}
	f.run(t)
	if f.sent.Load() != 1 {
		t.Fatal("accepted approved work replayed")
	}
}
