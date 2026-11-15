package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestClosureHistoryShowsApprovalMembershipWithoutPrivateAttempt(t *testing.T) {
	at := "2026-01-01T00:00:00.000Z"
	history := &noticeledger.RecoverySnapshot{Actions: []noticeledger.RecoveryAction{{
		Request: noticeledger.RecoveryRequest{RequestID: "approval-one", Kind: noticeledger.ApproveRevision,
			TargetID: strings.Repeat("a", 64), RoutingGeneration: strings.Repeat("b", 64), ReviewFingerprint: "private-review", ComponentFingerprint: "private-component"},
		Status: noticeledger.RecoveryApproved, RecordedAt: at, CompletedAt: &at, InvalidatedAt: &at, InvalidationReason: "source_changed",
		Attempt: &noticeledger.RecoveryAttempt{OwnerToken: "private-owner"}, IntentDigest: "private-digest", BatchIDs: []string{"first-batch", "second-batch"},
	}}}
	items := closureHistory(history)
	body, err := json.Marshal(items)
	if err != nil || strings.Contains(string(body), "private-") {
		t.Fatal("history exposed private authority", string(body), err)
	}
	if len(items) != 3 {
		t.Fatal("approval membership missing", items)
	}
	a := items[0].(closureActionItem)
	if a.Action != "APPROVE_REVISION" || a.Status != "APPROVED" || a.InvalidationReason != "source_changed" || a.InvalidatedAt == nil {
		t.Fatal("approval lifecycle lost", a)
	}
	for i, id := range []string{"first-batch", "second-batch"} {
		row := items[i+1].(closureActionBatchItem)
		if row.Kind != "ACTION_BATCH" || row.RequestID != "approval-one" || row.BatchID != id {
			t.Fatal("membership misattributed", row)
		}
	}
}

func TestClosureReviewRejectsMissingTargetAndInvalidStorageBinding(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..missing-target", 1, false)
	for _, kind := range []string{"CLOSE_BATCH", "CLOSE_REVISION"} {
		got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, kind, strings.Repeat("f", 64), ""))
		if got["status"] != "not-ready" || got["code"] != "closure_target_missing" || got["closureReview"] != nil {
			t.Fatal("missing target produced a review", got)
		}
	}
	p, failure := loadPinned(operationalContext(t), f.s, f.objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	p.environment = "invalid"
	ops, err := composeOperations(f.s, f.objects, p, "review-closure")
	if err == nil || ops.closureReview != nil {
		t.Fatal("unbound store exposed review capability")
	}
}
