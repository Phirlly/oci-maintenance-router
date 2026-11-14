package reconciliation

import (
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"testing"
)

func TestRecoveryClosedUnassignedRevisionCannotPrepareOnRestart(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	old := f.saved(t, n.Key().NoticeID)
	q := reviewedRecovery(t, old, noticeledger.CloseRevision, n.Key().Digest)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	if _, err := p.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal(err)
	}
	f.configure(t, func(v map[string]any) { v["schemaVersion"] = 7 })
	s := f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if s.Prepared != 0 || s.Accepted != 0 || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 || len(after.Batches) != 0 || after.Revisions[0].Resources[0].Disposition != "CLOSED" {
		t.Fatal("closed unassigned work became eligible", s)
	}
}

func TestRecoveryClosedRevisionCannotBeApprovedAgain(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	q := reviewedRecovery(t, f.saved(t, n.Key().NoticeID), noticeledger.CloseRevision, n.Key().Digest)
	p := recoveryOnlyProcessor(t, f, "PAUSED", q)
	if _, err := p.Run(leaseBudget(t), f.live.lease); err != nil {
		t.Fatal(err)
	}
	closed := f.saved(t, n.Key().NoticeID)
	q = reviewedRecovery(t, closed, noticeledger.ApproveRevision, n.Key().Digest)
	q["requestId"] = "new-approval"
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	s := f.run(t)
	if s.Recovery == nil || s.Recovery.Reason != "target_terminal" || f.current.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("closed revision did not remain explicitly terminal", s)
	}
}
