package reconciliation

import (
	"bytes"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func selectedReviewer(t *testing.T, f *processingFixture) *RecoveryReviewer {
	t.Helper()
	store, err := ocistorage.NewNoticeStore(f.live.objects, f.live.lease.manager.environmentID)
	if err != nil {
		t.Fatal(err)
	}
	clients := processingMySQL(t)
	clients.Compute = processingCompute(t).Compute
	r, err := NewRecoveryReviewer(f.live.config, []byte(resourceCatalogWire), candidateLocation(), f.source, store, clients)
	if err != nil {
		t.Fatal(err)
	}
	r.ownershipReaders, r.wait = f.p.ownershipReaders, f.p.wait
	return r
}

func TestSelectedRecoveryReviewMigrationMatchesExecution(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		n := selectedNotice(t, 1, "ACTION_REQUIRED", resourceInstance)
		f, setup := selectedProcessingFixture(t, []string{"instance"}, nil, n)
		old := candidateRecord(t, n)
		var err error
		switch version {
		case 1:
			old = discoveryLegacy(t, old)
		case 3:
			old, err = old.MigrateRecovery(leaseTime())
		case 4:
			old = announcementRecordFor(t, old)
		}
		if err != nil {
			t.Fatal(err)
		}
		before, _ := old.Encode()
		f.live.objects.seed(noticeName(n.Key().NoticeID), before)
		r := selectedReviewer(t, f)
		view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
		if err != nil || view.Blocker != "" || view.Fingerprint == "" || view.Projected.Snapshot().SchemaVersion != 4 {
			t.Fatal("selected migration review failed", version, view.Blocker, err)
		}
		after, _ := f.saved(t, n.Key().NoticeID).Encode()
		if !bytes.Equal(before, after) || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
			t.Fatal("review mutated or delivered")
		}
		q := reviewRequest(view, n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
		configureSelectedProcessing(t, f, setup, []string{"instance"}, func(v map[string]any) { v["recovery"] = q })
		got := f.run(t)
		if got.Recovery == nil || got.Recovery.Status != string(noticeledger.RecoveryApproved) || got.Accepted != 1 || f.sent.Load() != 1 {
			t.Fatal("review fingerprint did not authorize same projected work", got)
		}
		f.run(t)
		if f.sent.Load() != 1 || f.saved(t, n.Key().NoticeID).Snapshot().SchemaVersion != 4 {
			t.Fatal("recovery replayed or downgraded")
		}
	}
}

func TestLegacyRecoveryReviewerRejectsAnnouncementRecord(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	r := reviewFixture(t, f)
	old := announcementRecordFor(t, candidateRecord(t, n))
	b, _ := old.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), b)
	view, err := r.Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	if err == nil || view.Fingerprint != "" || strings.Contains(err.Error(), processingDB) {
		t.Fatal("old reviewer exposed incompatible authority", err)
	}
	after, _ := f.saved(t, n.Key().NoticeID).Encode()
	if !bytes.Equal(b, after) || f.sent.Load() != 0 || f.current.Load() != 0 {
		t.Fatal("review altered state")
	}
}
