package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"testing"
	"time"
)

func TestRecoveryRechecksAuthorityAndReadinessAfterCredentialRead(t *testing.T) {
	for _, approval := range []bool{false, true} {
		for _, fault := range []string{"cancel", "pins", "clock", "channel", "invalid-receipt"} {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			old := seedRecoveryRevision(t, f, n)
			kind, target := noticeledger.ApproveRevision, n.Key().Digest
			if !approval {
				old = f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
				kind, target = noticeledger.RetryBatch, old.Snapshot().Batches[0].Message.BatchID
			}
			q := reviewedRecovery(t, old, kind, target)
			f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
			ctx, cancel := context.WithCancel(leaseBudget(t))
			invalidate := func() {
				switch fault {
				case "cancel":
					cancel()
				case "pins":
					f.live.objects.seed("configuration/router.json", []byte("changed"))
				case "clock":
					f.live.lease.manager.now = func() time.Time { return time.Time{} }
				case "channel":
					f.p.selector.operator.read = func(context.Context) (OperatorChannel, error) {
						c := f.p.selector.operator.input.desired
						c.SubscriptionID = "replaced"
						return c, nil
					}
				case "invalid-receipt":
					f.live.objects.seed(operatorRunObjectName, []byte("private-invalid-state"))
				}
			}
			current, version := f.p.current, f.p.version
			f.p.current = func(c context.Context, id string) (maintenanceCredential, error) {
				key, err := current(c, id)
				invalidate()
				return key, err
			}
			f.p.version = func(c context.Context, id string, v int64) (maintenanceCredential, error) {
				key, err := version(c, id, v)
				invalidate()
				return key, err
			}
			r, err := f.p.Run(ctx, f.live.lease)
			cancel()
			if f.sent.Load() != 0 || r.Snapshot().Completed {
				t.Fatal("lost prerequisites dispatched/completed", approval, fault, err)
			}
			if fault == "channel" {
				if r.Snapshot().Recovery == nil || r.Snapshot().Recovery.Reason != "readiness_required" {
					t.Fatal("proven readiness change lost rejection", approval, err)
				}
			} else if err == nil || r.Snapshot().Recovery != nil {
				t.Fatal("operational failure fabricated a durable rejection", approval, fault, err)
			}
		}
	}
}
