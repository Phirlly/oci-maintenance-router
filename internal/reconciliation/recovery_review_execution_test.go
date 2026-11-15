package reconciliation

import (
	"bytes"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestRecoveryReviewProducesExecutableApprovalWithoutSaving(t *testing.T) {
	for _, kind := range []string{"singleton", "held-revision", "linked-peer"} {
		t.Run(kind, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			var old notices.Notice
			all := []notices.Notice{n}
			if kind == "held-revision" {
				old = n
				raw := n.Snapshot()
				raw.Summary = chain("Revised maintenance window")
				n = normalizeCandidate(t, raw)
				all = []notices.Notice{n}
			}
			if kind == "linked-peer" {
				raw := n.Snapshot()
				raw.ChainID = chain("linked")
				n = normalizeCandidate(t, raw)
				peer := processingNotice(t, 2, processingDB).Snapshot()
				peer.ChainID = chain("linked")
				all = []notices.Notice{n, normalizeCandidate(t, peer)}
			}
			f := processingFixtureWithBaseline(t, all, all...)
			if old.Key().NoticeID != "" {
				record := seedReviewRevision(t, f, old, n)
				if old.Key().Digest == n.Key().Digest || len(record.Snapshot().Revisions) != 2 || record.Snapshot().HoldReason != delivery.ReasonSourceChanged {
					t.Fatal("fixture did not retain distinct held revisions")
				}
			} else {
				seedReviewRevision(t, f, n)
			}
			r := reviewFixture(t, f)
			before, _ := f.saved(t, n.Key().NoticeID).Encode()
			view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
			after, _ := f.saved(t, n.Key().NoticeID).Encode()
			if err != nil || view.Blocker != "" || view.Fingerprint == "" || !bytes.Equal(before, after) || f.sent.Load() != 0 || f.current.Load() != 0 || f.exact.Load() != 0 {
				t.Fatal("review changed work or failed", view.Blocker, err)
			}
			if len(view.Routing.Decisions) != 1 || view.Routing.Decisions[0].Disposition != routing.Ready || view.Routing.Decisions[0].DestinationID != "payments" {
				t.Fatal("current routing missing", view.Routing)
			}
			if kind == "linked-peer" && len(view.Component.Members) != 2 {
				t.Fatal("linked peer missing")
			}
			q := reviewRequest(view, n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
			f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
			got := f.run(t)
			if got.Recovery == nil || got.Recovery.Status != "APPROVED" || got.Accepted != 1 || f.sent.Load() != 1 {
				t.Fatal("review does not match actual coordinator", got)
			}
			f.run(t)
			if f.sent.Load() != 1 {
				t.Fatal("same reviewed action resent")
			}
		})
	}
}

func TestRecoveryReviewProducesExactManualRetry(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureWithBaseline(t, []notices.Notice{n}, n)
	old := f.seedBatch(t, n, time.Now().Add(-25*time.Hour), delivery.RetryWait, true)
	r := reviewFixture(t, f)
	id := old.Snapshot().Batches[0].Message.BatchID
	view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.RetryBatch, id)
	if err != nil || view.Blocker != "" || view.Fingerprint == "" {
		t.Fatal("retry review", view.Blocker, err)
	}
	q := reviewRequest(view, n.Key().NoticeID, noticeledger.RetryBatch, id)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	got := f.run(t)
	if got.Recovery == nil || got.Recovery.Status != "API_ACCEPTED" || f.sent.Load() != 1 || f.current.Load() != 0 || f.exact.Load() != 1 {
		t.Fatal("reviewed retry failed", got)
	}
}
