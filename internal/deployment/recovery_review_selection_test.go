package deployment

import (
	"encoding/json"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestRecoveryReviewKeepsSelectedBatchAndCurrentOwnershipSeparate(t *testing.T) {
	prepared := preparedClosureFixture(t)
	f, view := selectedRecoveryFixture(t, prepared.notice.Key().NoticeID, 2)
	view.Saved, view.Projected = prepared.record, prepared.record
	batch := prepared.record.Snapshot().Batches[0]
	for _, action := range []string{"RETRY_BATCH", "APPROVE_REVISION", ""} {
		q := command{noticeID: f.notice.Key().NoticeID, kind: action, targetID: f.notice.Key().Digest}
		if action == "RETRY_BATCH" {
			q.targetID = batch.Message.BatchID
		}
		// Prepared batches are ineligible for a manual retry; preserve the blocker
		// supplied by the domain reviewer instead of using closure eligibility.
		view.Blocker = "target_not_review_required"
		page, err := projectRecoveryReview(f.s, view, q)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(page.Items)
		var rows []map[string]any
		_ = json.Unmarshal(b, &rows)
		batches, routes := 0, 0
		for _, row := range rows {
			switch row["kind"] {
			case "SAVED_BATCH":
				batches++
				if action == "RETRY_BATCH" && row["batchId"] != batch.Message.BatchID {
					t.Fatal("wrong batch")
				}
				if row["blocker"] != nil && row["blocker"] != "" {
					t.Fatal("closure eligibility leaked")
				}
			case "CURRENT_ROUTING":
				routes++
				if action == "APPROVE_REVISION" && row["eligible"] != false {
					t.Fatal("assigned resource reapproved")
				}
			case "RECOVERY":
				t.Fatal("blocked target authorized")
			}
		}
		wantB, wantR := 2, 2
		if action == "RETRY_BATCH" {
			wantB, wantR = 1, 1
		}
		if action == "" {
			wantR = 0
		}
		if batches != wantB || routes != wantR {
			t.Fatal("lost selected evidence", action, batches, routes)
		}
	}
	if page, err := projectRecoveryReview(f.s, reconciliation.RecoveryReview{}, command{}); err == nil || page != nil {
		t.Fatal("empty record projected")
	}
	view.Blocker, view.Fingerprint = "", ""
	if page, err := projectRecoveryReview(f.s, view, command{kind: "APPROVE_REVISION", targetID: f.notice.Key().Digest}); err == nil || page != nil {
		t.Fatal("missing fingerprint became authority")
	}
}
