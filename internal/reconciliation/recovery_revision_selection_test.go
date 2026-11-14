package reconciliation

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/routing"
	"reflect"
	"testing"
	"time"
)

func TestRecoveryApprovesExactCurrentRevisionRegardlessOfDigestOrder(t *testing.T) {
	for _, first := range []bool{false, true} {
		a := processingNotice(t, 1, processingDB)
		v := a.Snapshot()
		v.Reference = chain("updated-maintenance")
		b := normalizeCandidate(t, v)
		current, previous := a, b
		if (a.Key().Digest < b.Key().Digest) != first {
			current, previous = b, a
		}
		f := processingFixtureFor(t, current)
		old := f.seedBatch(t, previous, time.Now().Add(-time.Minute), delivery.APIAccepted, false)
		accepted := old.Snapshot().Batches[0]
		m, err := f.p.mysql["us-ashburn-1"](leaseBudget(t), processingDB)
		if err != nil {
			t.Fatal(err)
		}
		retained, err := f.p.observer.Observe(old, current, true, map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: m}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		body, _ := retained.Encode()
		f.live.objects.seed(noticeName(current.Key().NoticeID), body)
		q := reviewedRecovery(t, retained, noticeledger.ApproveRevision, current.Key().Digest)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		s := f.run(t)
		after := f.saved(t, current.Key().NoticeID).Snapshot()
		if s.Accepted != 1 || f.sent.Load() != 1 || s.Recovery == nil || s.Recovery.Status != string(noticeledger.RecoveryApproved) || len(after.Batches) != 2 || len(after.Recovery.Actions[0].BatchIDs) != 1 {
			t.Fatal("wrong exact revision approved", first, s)
		}
		for _, got := range after.Batches {
			if got.Message.Revision == previous.Key() && !reflect.DeepEqual(got, accepted) {
				t.Fatal("accepted prior revision changed")
			}
			if got.Message.Revision == current.Key() && got.State != delivery.APIAccepted {
				t.Fatal("current approved work was blocked by history")
			}
		}
		f.run(t)
		if f.sent.Load() != 1 {
			t.Fatal("approved revision replayed")
		}
	}
}

func TestRecoveryApprovedPeerContinuesInUnchangedMultiNoticeComponent(t *testing.T) {
	f, peer, _ := linkedRecoveryFixture(t)
	f.configure(t, func(v map[string]any) { v["schemaVersion"] = 7 })
	s := f.run(t)
	a := recoveryActions(t, f, peer.Key().NoticeID)[0]
	if s.Accepted != 1 || f.sent.Load() != 1 || a.InvalidatedAt != nil || f.saved(t, peer.Key().NoticeID).Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("unchanged approved component did not continue", s)
	}
}
