package deployment

import (
	"reflect"
	"strings"
	"testing"
)

func TestSavedReviewBypassesEveryModeSpecificProducer(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		t.Run(mode, func(t *testing.T) {
			f := newSavedReviewFixture(t, "ocid1.announcement.oc1..example", 1, false)
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
			f.h.connect = func(Settings) (Store, error) { return &managedStore{f.objects, principal}, nil }
			before, writes := businessObjects(f.objects), f.objects.version
			got := invokeSavedReview(t, f.h, f.notice.Key().NoticeID, "")
			if got.Status != "reviewed" || got.Review == nil || principal.reads != 0 || !reflect.DeepEqual(before, businessObjects(f.objects)) || f.objects.version != writes+2 {
				t.Fatalf("review crossed mode boundary: %+v", got)
			}
		})
	}
}

func TestSavedReviewCompositionRequiresValidStorageBinding(t *testing.T) {
	_, s, objects, _ := operationalFixture(t)
	p, failure := loadPinned(operationalContext(t), s, objects)
	if failure.Status != "" {
		t.Fatal(failure)
	}
	p.environment = "invalid"
	ops, err := composeOperations(s, objects, p, "review-notice")
	if err == nil || ops.review != nil {
		t.Fatal("invalid environment produced review reader")
	}
}
