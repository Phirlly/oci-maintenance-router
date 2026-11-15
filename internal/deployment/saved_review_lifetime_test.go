package deployment

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestSavedReviewCoalescesWithoutReadingNoticeOrChangingOtherOwner(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
	manager, err := reconciliation.NewLeaseManager(f.objects, f.environment)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := manager.Acquire(operationalContext(t))
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	writes := f.objects.version
	f.objects.before = func(_ context.Context, method, name string) error {
		if method == "GET" && strings.HasPrefix(name, "notices/") {
			t.Fatal("overlap read notice")
		}
		return nil
	}
	got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
	if got.Status != "coalesced" || got.Review != nil || f.objects.version != writes {
		t.Fatalf("overlap produced a review: %+v", got)
	}
	if err := lease.Check(operationalContext(t), time.Second); err != nil {
		t.Fatal("other owner changed", err)
	}
}

func TestSavedReviewCancellationDiscardsOutputAndReleasesLease(t *testing.T) {
	f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
	ctx, cancel := context.WithCancel(operationalContext(t))
	defer cancel()
	writes := f.objects.version
	f.objects.before = func(_ context.Context, method, name string) error {
		if method == "GET" && strings.HasPrefix(name, "review/preparation/") {
			cancel()
			return ctx.Err()
		}
		return nil
	}
	out := httptest.NewRecorder()
	f.h.Serve(ctx, strings.NewReader(reviewInvocation(f.notice.Key().NoticeID, "")), out)
	var got OperationalResult
	if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != "not-ready" || got.Code != "canceled" || got.Review != nil || got.CleanupCode != "" || f.objects.version != writes+2 {
		t.Fatalf("canceled review survived or abandoned ownership: %s", out.Body)
	}
}

func TestSavedReviewRejectsLegacyConfigurationBeforeOperationalWork(t *testing.T) {
	h, _, store := handlerFixture(t)
	out := httptest.NewRecorder()
	h.Serve(testContext(t), strings.NewReader(reviewInvocation("ocid1.announcement.oc1..example", "")), out)
	if !strings.Contains(out.Body.String(), `"code":"configuration_unsupported"`) || store.creates != 0 {
		t.Fatalf("legacy installation performed review: %s", out.Body)
	}
}
