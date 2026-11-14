package reconciliation

import (
	"context"
	"strings"
	"testing"
)

func TestOperatorReceiptRejectsInvalidCallsBeforeIO(t *testing.T) {
	for _, kind := range []string{"nil-verifier", "zero-verifier", "nil-lease", "foreign-lease", "nil-context", "no-deadline", "canceled"} {
		f := operatorCoordinatorFixtureFor(t)
		v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
		lease, ctx := f.lease, leaseBudget(t)
		want := "invalid_request"
		switch kind {
		case "nil-verifier":
			v = nil
		case "zero-verifier":
			v = &OperatorReceiptVerifier{}
		case "nil-lease":
			lease = nil
		case "foreign-lease":
			lease.manager.environmentID = strings.Repeat("a", 64)
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
		}
		before := len(f.objects.calls)
		got, err := v.Verify(ctx, lease)
		operatorReceiptFailure(t, got, err, want)
		if len(f.objects.calls) != before {
			t.Fatal("invalid verification reached backend", kind)
		}
	}
}
