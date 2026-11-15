package deployment

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestRecoveryReviewBoundsCompleteCurrentOwnershipAndMaximumCode(t *testing.T) {
	prefix := "ocid1.announcement.oc1.."
	id := prefix + strings.Repeat("a", 4096-len(prefix))
	f, view := selectedRecoveryFixture(t, id, 80)
	bindRecoveryView(f, &view)
	before := businessObjects(f.objects)
	rows := recoveryPages(t, f, "APPROVE_REVISION", f.notice.Key().Digest)
	seen := map[string]bool{}
	for _, row := range rows {
		if row["kind"] == "CURRENT_ROUTING" {
			id := row["resourceId"].(string)
			if seen[id] || row["eligible"] != true || row["destinationId"] != "payments" {
				t.Fatal("missing or duplicated current ownership", row)
			}
			seen[id] = true
		}
		body, _ := json.Marshal(row)
		for _, private := range []string{"private-reference", "vaultsecret", "ownerToken", "receiptCode", "payload"} {
			if strings.Contains(string(body), private) {
				t.Fatal("private data in review", private)
			}
		}
	}
	if len(seen) != 80 || rows[len(rows)-1]["kind"] != "RECOVERY" || len(rows[len(rows)-1]["code"].(string)) > 6000 || !reflect.DeepEqual(before, businessObjects(f.objects)) {
		t.Fatal("review incomplete, oversize or mutated storage")
	}
}

func TestRecoveryPagingRejectsChangedObservedOrSavedContext(t *testing.T) {
	for _, kind := range []string{"saved", "routing", "component", "configuration", "catalog", "release", "target", "blocker"} {
		t.Run(kind, func(t *testing.T) {
			f, view := selectedRecoveryFixture(t, "ocid1.announcement.oc1..paging", 80)
			bindRecoveryView(f, &view)
			q := command{operation: "review-recovery", noticeID: f.notice.Key().NoticeID, kind: "APPROVE_REVISION", targetID: f.notice.Key().Digest}
			first, err := projectRecoveryReview(f.s, view, q)
			if err != nil || first.NextCursor == "" {
				t.Fatal(err)
			}
			q.cursor = first.NextCursor
			switch kind {
			case "saved":
				view.Saved, err = view.Saved.Hold("source_changed", reviewTime())
				if err != nil {
					t.Fatal(err)
				}
			case "routing":
				view.Routing.Decisions[0].DestinationID = "other"
			case "component":
				view.Component.Fingerprint = strings.Repeat("d", 64)
			case "configuration":
				f.s.ConfigurationSHA256 = strings.Repeat("d", 64)
			case "catalog":
				f.s.CatalogSHA256 = strings.Repeat("d", 64)
			case "release":
				f.s.ReleaseSHA256 = strings.Repeat("d", 64)
			case "target":
				q.targetID = strings.Repeat("d", 64)
			case "blocker":
				view.Blocker = "source_changed"
			}
			page, err := projectRecoveryReview(f.s, view, q)
			if page != nil || err == nil || operationCode(operationalContext(t), err) != "review_changed" {
				t.Fatal("drift continued review", kind, err)
			}
		})
	}
}

func TestRecoveryPageRefusesBadCursorAndOversizedEvidence(t *testing.T) {
	binding := strings.Repeat("a", 64)
	for _, cursor := range []string{"bad", binding + ":0", binding + ":01", binding + ":2"} {
		if page, err := recoveryReviewPage(noticeledger.Snapshot{}, []any{"item"}, binding, command{cursor: cursor}); page != nil || err == nil {
			t.Fatal("invalid cursor accepted", cursor)
		}
	}
	if page, err := recoveryReviewPage(noticeledger.Snapshot{}, []any{strings.Repeat("x", 6500)}, binding, command{}); page != nil || err == nil {
		t.Fatal("oversize item truncated")
	}
	f, view := selectedRecoveryFixture(t, "ocid1.announcement.oc1..code", 1)
	if _, err := recoveryReviewCode(view, command{kind: strings.Repeat("x", 6000), targetID: f.notice.Key().Digest}); err == nil {
		t.Fatal("oversize code allowed")
	}
}
