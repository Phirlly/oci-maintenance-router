package deployment

import (
	"context"
	"strings"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
	"oci-maintenance-router/internal/routing"
)

func selectedRecoveryFixture(t *testing.T, id string, count int) (savedReviewFixture, reconciliation.RecoveryReview) {
	t.Helper()
	f := newSavedReviewFixture(t, id, count, true)
	f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) {
		v["mode"] = "LIVE"
		v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true, "receiptCode": "OMR-" + strings.Repeat("a", 32)}
		v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid", "receiptCode": "OMR-OP-" + strings.Repeat("a", 32)}
	})
	p, failure := loadPinned(operationalContext(t), f.s, f.objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	policy, err := routing.New(p.configBytes, p.catalog)
	if err != nil {
		t.Fatal(err)
	}
	record, err := projectClosureRecord(f.record)
	if err != nil {
		t.Fatal(err)
	}
	view := reconciliation.RecoveryReview{Saved: record, Projected: record, Routing: policy.Resolve(f.notice, true, f.lookups), Fingerprint: strings.Repeat("b", 64), Component: reconciliation.CandidateComponent{Fingerprint: strings.Repeat("c", 64), Members: []reconciliation.CandidateRevision{{Revision: f.notice.Key()}}}}
	return f, view
}

func bindRecoveryView(f savedReviewFixture, view *reconciliation.RecoveryReview) {
	f.h.compose = func(s Settings, _ Store, _ pinnedInputs, _ string) (operations, error) {
		return operations{recoveryReview: func(_ context.Context, q command) (*RecoveryReviewPage, error) {
			return projectRecoveryReview(s, *view, q)
		}}, nil
	}
}

func recoveryPages(t *testing.T, f savedReviewFixture, kind, target string) []map[string]any {
	t.Helper()
	cursor := ""
	items := []map[string]any{}
	for count := 0; count < 100; count++ {
		got := closureResponse(t, f.h, recoveryReviewRequest(f.notice.Key().NoticeID, kind, target, cursor))
		if got["status"] != "reviewed" {
			t.Fatal(got)
		}
		page := got["recoveryReview"].(map[string]any)
		rows := page["items"].([]any)
		if len(rows) == 0 || len(rows) > 32 {
			t.Fatal("unbounded or stalled review")
		}
		for _, value := range rows {
			items = append(items, value.(map[string]any))
		}
		cursor, _ = page["nextCursor"].(string)
		if cursor == "" {
			if len(items) != int(page["totalItems"].(float64)) {
				t.Fatal("lost items")
			}
			return items
		}
		for _, row := range rows {
			if row.(map[string]any)["kind"] == "RECOVERY" {
				t.Fatal("code before complete review")
			}
		}
	}
	t.Fatal("pagination did not terminate")
	return nil
}
