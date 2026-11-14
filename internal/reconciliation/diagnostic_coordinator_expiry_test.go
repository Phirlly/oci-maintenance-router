package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticCoordinatorExpiryRequiresCurrentAuthorityAndCAS(t *testing.T) {
	for _, kind := range []string{"changed-controls", "denied", "future-record"} {
		f := coordinatorFixture(t)
		f.seed(t, DiagnosticSending, true)
		want := "write_failed"
		if kind == "changed-controls" {
			want = "configuration_changed"
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.method == "GET" && c.name == diagnosticObjectName {
					f.objects.seed("configuration/router.json", []byte("changed"))
				}
				return nil
			}
		} else if kind == "denied" {
			f.objects.before = func(_ context.Context, c discoveryCall) error {
				if c.method == "REPLACE" && c.name == diagnosticObjectName {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
				return nil
			}
		} else {
			want = "invalid_time"
			now := f.lease.manager.now().Add(time.Minute)
			r, err := NewDiagnostic(f.coordinator.intent, 9, DiagnosticClaim{f.lease.record.OwnerToken, newLeaseToken(), now, now.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := r.Encode()
			f.objects.seed(diagnosticObjectName, body)
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		diagnosticRunFailure(t, err, want)
		if got.Dispatched() || got.Record().Snapshot().State != DiagnosticSending || f.saved(t).Snapshot().State != DiagnosticSending || f.reads.Load() != 0 || f.sends.Load() != 0 {
			t.Fatal("expiry bypassed authority or reported unsaved state")
		}
	}
}

func TestDiagnosticCoordinatorExpiredCallerNeverStartsWork(t *testing.T) {
	f := coordinatorFixture(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	before := len(f.objects.calls)
	got, err := f.coordinator.Run(ctx, f.lease)
	e := diagnosticRunFailure(t, err, "deadline_exceeded")
	if e.Unwrap() != context.DeadlineExceeded || got.Dispatched() || len(f.objects.calls) != before {
		t.Fatal("expired caller started work")
	}
}
