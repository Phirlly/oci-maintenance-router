package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
)

func TestMaintenanceRechecksReadinessBeforeEveryAttempt(t *testing.T) {
	for _, stage := range []string{"prepared", "retry"} {
		t.Run(stage, func(t *testing.T) {
			n := processingNotice(t, 1, processingDB)
			f := processingFixtureFor(t, n)
			invalidate := func() { f.live.objects.seed(diagnosticObjectName, []byte("invalid receipt")) }
			if stage == "prepared" {
				current := f.p.current
				f.p.current = func(ctx context.Context, id string) (maintenanceCredential, error) {
					key, err := current(ctx, id)
					invalidate()
					return key, err
				}
			} else {
				send := f.p.senders["US"]
				f.p.senders["US"] = func(ctx context.Context, body, key []byte) (pagerduty.Response, error) {
					response, err := send(ctx, body, key)
					response.Status = 503
					invalidate()
					return response, err
				}
			}
			r, err := f.p.Run(leaseBudget(t), f.live.lease)
			wantState, wantSends := delivery.Prepared, int32(0)
			if stage == "retry" {
				wantState, wantSends = delivery.RetryWait, 1
			}
			if err == nil || r.Snapshot().Completed || r.Snapshot().Accepted != 0 || f.sent.Load() != wantSends {
				t.Fatal("changed readiness permitted another attempt", err)
			}
			if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != wantState {
				t.Fatal("readiness failure discarded pending work")
			}
		})
	}
}
