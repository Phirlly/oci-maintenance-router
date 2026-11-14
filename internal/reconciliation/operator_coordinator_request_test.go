package reconciliation

import (
	"context"
	"strings"
	"testing"
)

func TestOperatorCoordinatorRejectsInvalidRunBeforeIO(t *testing.T) {
	for _, kind := range []string{"nil-owner", "zero-owner", "nil-lease", "foreign-lease", "nil-context", "no-deadline", "canceled", "token", "owner-token"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			d, l, ctx := f.coordinator, f.lease, leaseBudget(t)
			want := "invalid_request"
			switch kind {
			case "nil-owner":
				d = nil
			case "zero-owner":
				d = &OperatorCoordinator{}
			case "nil-lease":
				l = nil
			case "foreign-lease":
				l.manager.environmentID = strings.Repeat("a", 64)
				want = "environment_mismatch"
			case "nil-context":
				ctx = nil
			case "no-deadline":
				ctx = context.Background()
				want = "deadline_required"
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = "canceled"
			case "token":
				d.token = func() string { return "invalid" }
			case "owner-token":
				d.token = func() string { return l.record.OwnerToken }
			}
			before := len(f.objects.calls)
			got, err := d.Run(ctx, l)
			operatorRunFailure(t, err, want)
			if got.Dispatched() || got.Record().Snapshot().State != "" || len(f.objects.calls) != before || f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("invalid run had effects")
			}
		})
	}
}
