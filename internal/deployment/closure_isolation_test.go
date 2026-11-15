package deployment

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestClosureReviewNeverExecutesModeOrConfiguredRecovery(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		f := newSavedReviewFixture(t, "ocid1.announcement.oc1..mode", 1, false)
		intent := reviewedClosureIntent(t, f, "waiting-close")
		f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) {
			v["mode"] = mode
			v["recovery"] = intent
			v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true, "receiptCode": "OMR-" + strings.Repeat("a", 32)}
			v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid", "receiptCode": "OMR-OP-" + strings.Repeat("b", 32)}
		})
		principal := &unusedPrincipal{}
		f.h.connect = func(Settings) (Store, error) { return &managedStore{f.objects, principal}, nil }
		before := businessObjects(f.objects)
		got := closureResponse(t, f.h, closureReviewRequest(f.notice.Key().NoticeID, "", "", ""))
		if got["status"] != "reviewed" || principal.reads != 0 || !reflect.DeepEqual(before, businessObjects(f.objects)) {
			t.Fatal("inspection crossed mode boundary", mode, got)
		}
	}
}

func TestClosureReviewCoalescesAndCancellationDiscardsCode(t *testing.T) {
	for _, kind := range []string{"overlap", "cancel"} {
		f := newSavedReviewFixture(t, "ocid1.announcement.oc1..lifetime", 1, false)
		ctx, cancel := context.WithCancel(operationalContext(t))
		defer cancel()
		if kind == "overlap" {
			manager, err := reconciliation.NewLeaseManager(f.objects, f.environment)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := manager.Acquire(ctx)
			if err != nil || lease == nil {
				t.Fatal(err)
			}
		}
		f.objects.before = func(_ context.Context, method, name string) error {
			if method == "GET" && strings.HasPrefix(name, "notices/") {
				if kind == "overlap" {
					t.Fatal("overlapping inspection read notice")
				}
				cancel()
				return ctx.Err()
			}
			return nil
		}
		out := httptest.NewRecorder()
		f.h.Serve(ctx, strings.NewReader(closureReviewRequest(f.notice.Key().NoticeID, "CLOSE_REVISION", f.notice.Key().Digest, "")), out)
		var got OperationalResult
		if json.Unmarshal(out.Body.Bytes(), &got) != nil {
			t.Fatal(out.Body)
		}
		if got.ClosureReview != nil || got.Status == "reviewed" || kind == "overlap" && got.Status != "coalesced" || kind == "cancel" && (got.Code != "canceled" || got.CleanupCode != "") {
			t.Fatal("failed lifetime retained code", got)
		}
	}
}
