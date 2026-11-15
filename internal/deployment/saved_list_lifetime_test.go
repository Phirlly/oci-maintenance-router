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

func TestSavedListCoalescesWithoutReadingInventory(t *testing.T) {
	f, objects, _, _ := newSavedListFixture(t, 1)
	manager, err := reconciliation.NewLeaseManager(f.objects, f.environment)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := manager.Acquire(operationalContext(t))
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	writes := f.objects.version
	got := invokeSavedList(t, f.h, "")
	if got.Status != "coalesced" || got.SavedNotices != nil || len(objects.starts) != 0 || f.objects.version != writes {
		t.Fatalf("overlap changed owner or listed records: %+v", got)
	}
	if err := lease.Check(operationalContext(t), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestSavedListCancellationDropsOutputAndReleasesLease(t *testing.T) {
	for _, stage := range []string{"LIST", "GET"} {
		t.Run(stage, func(t *testing.T) {
			f, _, _, _ := newSavedListFixture(t, 1)
			ctx, cancel := context.WithCancel(operationalContext(t))
			defer cancel()
			writes := f.objects.version
			f.objects.before = func(_ context.Context, method, name string) error {
				if method == stage && strings.HasPrefix(name, "notices/") {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			out := httptest.NewRecorder()
			f.h.Serve(ctx, strings.NewReader(listInvocation("")), out)
			var got OperationalResult
			if json.Unmarshal(out.Body.Bytes(), &got) != nil || got.Status != "not-ready" || got.Code != "canceled" || got.SavedNotices != nil || got.CleanupCode != "" || f.objects.version != writes+2 {
				t.Fatalf("canceled listing survived or abandoned owner: %s", out.Body)
			}
		})
	}
}

func TestSavedListRejectsLegacyConfiguration(t *testing.T) {
	h, _, store := handlerFixture(t)
	out := httptest.NewRecorder()
	h.Serve(testContext(t), strings.NewReader(listInvocation("")), out)
	if !strings.Contains(out.Body.String(), `"code":"configuration_unsupported"`) || store.creates != 0 {
		t.Fatalf("legacy installation listed records: %s", out.Body)
	}
}
