package deployment

import (
	"context"
	"testing"
)

func TestSelectedRecoveryReviewRequiresTrustedReadOnlyBindings(t *testing.T) {
	for _, kind := range []string{"valid-canceled", "unmanaged", "nil-principal", "environment", "region", "mysql", "configuration"} {
		t.Run(kind, func(t *testing.T) {
			f := reviewLiveFixture(t)
			_, _, _, _, principal := retainedTestFixture(t, false)
			p, failure := loadPinned(operationalContext(t), f.s, f.objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			var store Store = &managedStore{f.objects, principal}
			switch kind {
			case "unmanaged":
				store = f.objects
			case "nil-principal":
				store = &managedStore{operationalObjects: f.objects}
			case "environment":
				p.environment = "invalid"
			case "region":
				f.s.Region = "../invalid"
			case "mysql":
				p.config.RegionMap["us-ashburn-1"] = "invalid.key"
			case "configuration":
				p.configBytes = []byte("invalid")
			}
			ops, err := composeOperations(f.s, store, p, "review-recovery")
			if kind == "environment" {
				if err == nil || ops.recoveryReview != nil {
					t.Fatal("invalid storage accepted")
				}
				return
			}
			if err != nil || ops.process != nil || ops.diagnostic != nil || ops.operator != nil || ops.selectLive != nil || ops.prepare != nil || ops.reportRetry != nil {
				t.Fatal("review composed authority", err)
			}
			ctx, cancel := context.WithCancel(operationalContext(t))
			if kind == "valid-canceled" {
				cancel()
			}
			defer cancel()
			before := f.objects.version
			page, err := ops.recoveryReview(ctx, command{operation: "review-recovery", noticeID: f.notice.Key().NoticeID, kind: "APPROVE_REVISION", targetID: f.notice.Key().Digest})
			if err == nil || page != nil || principal.reads != 0 || f.objects.version != before {
				t.Fatal("failed binding read credentials or changed state", kind, err)
			}
		})
	}
}
