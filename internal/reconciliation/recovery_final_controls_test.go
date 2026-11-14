package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

func TestRecoveryFinalReadRequiresCurrentAuthorityAndExactIntent(t *testing.T) {
	for _, fault := range []string{"pins", "read", "same-intent", "changed-intent"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
		q := reviewedRecovery(t, old, noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID)
		f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
		now, calls, final := f.p.discovery.now, 0, false
		f.p.discovery.now = func() time.Time {
			at := now()
			calls++
			if calls == 4 {
				final = true
				switch fault {
				case "pins":
					f.live.objects.seed("configuration/router.json", []byte("changed"))
				case "same-intent", "changed-intent":
					request := (&processingRun{owner: f.p}).recoveryRequest()
					if fault == "changed-intent" {
						request.Kind, request.ComponentFingerprint = noticeledger.CloseBatch, ""
					}
					next, _, err := f.saved(t, n.Key().NoticeID).RejectRecovery(request, "no_eligible_work", at)
					if err != nil {
						t.Fatal(err)
					}
					body, _ := next.Encode()
					f.live.objects.seed(noticeName(n.Key().NoticeID), body)
				}
			}
			return at
		}
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if final && fault == "read" && c.method == "GET" && c.name == noticeName(n.Key().NoticeID) {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if !final || f.sent.Load() != 0 || f.exact.Load() != 0 {
			t.Fatal("final recovery boundary bypassed", fault, err)
		}
		if fault == "same-intent" {
			if err != nil || r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "no_eligible_work" {
				t.Fatal("concurrently retained same intent was repeated", err)
			}
		} else if err == nil || r.Snapshot().Completed || r.Snapshot().Recovery != nil {
			t.Fatal("changed final authority/intent accepted", fault, err)
		}
	}
}
