package reconciliation

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestSelectedProcessingNeverReopensLegacyRedactedClosedResource(t *testing.T) {
	n := selectedNotice(t, 1, "ACTION_REQUIRED", resourceInstance)
	f, _ := selectedProcessingFixture(t, []string{"instance"}, nil, n)
	old, err := candidateRecord(t, n).MigrateRecovery(leaseTime())
	if err != nil {
		t.Fatal(err)
	}
	generation := strings.Repeat("a", 64)
	fingerprint, err := old.ReviewFingerprint(noticeledger.CloseRevision, n.Key().Digest, generation, "")
	if err != nil {
		t.Fatal(err)
	}
	closed, action, err := old.CloseRecovery(noticeledger.RecoveryRequest{RequestID: "closed-before-support", Kind: noticeledger.CloseRevision, TargetID: n.Key().Digest, ReviewFingerprint: fingerprint, RoutingGeneration: generation}, leaseTime())
	if err != nil || action.Status != noticeledger.RecoveryClosed {
		t.Fatal(err)
	}
	row := closed.Snapshot().Revisions[0].Resources[0]
	if row.ResourceID != "" || row.Disposition != "CLOSED" {
		t.Fatal("fixture not redacted closed work", row)
	}
	body, _ := closed.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	got := f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot()
	if got.Accepted != 0 || len(after.Batches) != 0 || !reflect.DeepEqual(after.Revisions[0].Resources[0], row) || f.current.Load() != 0 || f.exact.Load() != 0 || f.sent.Load() != 0 {
		t.Fatal("closed row created new delivery work", got)
	}
	view, err := selectedReviewer(t, f).Review(leaseBudget(t), n.Key().NoticeID, noticeledger.ApproveRevision, n.Key().Digest)
	if err != nil || view.Blocker != "target_terminal" {
		t.Fatal("closed resource advertised as eligible", view.Blocker, err)
	}
}
