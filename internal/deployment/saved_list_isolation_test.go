package deployment

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/noticeledger"
)

func TestSavedListBypassesEveryModeSpecificProducer(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		t.Run(mode, func(t *testing.T) {
			f, objects, _, _ := newSavedListFixture(t, 1)
			f.s = configureOperational(t, f.h, f.s, f.objects, func(v map[string]any) {
				v["mode"] = mode
				v["diagnostic"] = map[string]any{"requestId": "test-1", "secretId": "ocid1.vaultsecret.oc1.iad.diagnostic", "nonProduction": true}
				v["operatorTest"] = map[string]any{"requestId": "operator-1", "subscriptionId": "ocid1.onssubscription.oc1.iad.operator", "endpoint": "operator@example.invalid"}
				if mode == "LIVE" {
					v["diagnostic"].(map[string]any)["receiptCode"] = "OMR-" + strings.Repeat("a", 32)
					v["operatorTest"].(map[string]any)["receiptCode"] = "OMR-OP-" + strings.Repeat("b", 32)
				}
			})
			principal := &unusedPrincipal{}
			f.h.connect = func(Settings) (Store, error) { return &managedStore{objects, principal}, nil }
			before, writes := businessObjects(f.objects), f.objects.version
			f.objects.before = func(_ context.Context, method, name string) error {
				if method == "GET" && (strings.HasPrefix(name, "review/") || strings.HasPrefix(name, "tests/")) || method == "HISTORY" {
					t.Fatal("listing read unrelated records")
				}
				return nil
			}
			got := invokeSavedList(t, f.h, "")
			if got.Status != "listed" || principal.reads != 0 || !reflect.DeepEqual(before, businessObjects(f.objects)) || f.objects.version != writes+2 {
				t.Fatalf("listing crossed mode boundary: %+v", got)
			}
		})
	}
}

func TestSavedListDoesNotExecuteConfiguredRecovery(t *testing.T) {
	h, s, objects, env := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	_, _, id := configureRecovery(t, h, s, objects, env, noticeledger.CloseRevision)
	before, writes := businessObjects(objects), objects.version
	got := invokeSavedList(t, h, "")
	if got.Status != "listed" || !reflect.DeepEqual(got.SavedNotices.NoticeIDs, []string{id}) || !reflect.DeepEqual(before, businessObjects(objects)) || objects.version != writes+2 {
		t.Fatalf("listing executed recovery: %+v", got)
	}
}

func TestSavedListCompositionRejectsInvalidStorageEnvironment(t *testing.T) {
	_, s, objects, _ := operationalFixture(t)
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	p.environment = "invalid"
	ops, err := composeOperations(s, objects, p, "list-notices")
	if err == nil || ops.list != nil {
		t.Fatal("invalid environment produced listing reader")
	}
}
