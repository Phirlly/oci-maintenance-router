package reconciliation

import (
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
)

func TestDiagnosticCoordinatorPreserves202AfterLostWorkAuthority(t *testing.T) {
	for _, change := range []string{"configuration", "catalog", "lease", "run-cancel", "origin-cancel"} {
		t.Run(change, func(t *testing.T) {
			origin, stopOrigin := context.WithCancel(leaseBudget(t))
			defer stopOrigin()
			f := coordinatorFixture(t, origin)
			ctx, stopRun := context.WithCancel(leaseBudget(t))
			defer stopRun()
			sent := false
			outcomes := 0
			f.objects.before = func(c context.Context, call discoveryCall) error {
				if sent {
					if call.name != diagnosticObjectName || call.method != "REPLACE" || c.Err() != nil {
						t.Error("receipt reused new-send guards/cancellation")
					}
					outcomes++
				}
				return nil
			}
			f.coordinator.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
				sent = true
				f.sends.Add(1)
				switch change {
				case "configuration":
					f.objects.seed("configuration/router.json", []byte("changed"))
				case "catalog":
					f.objects.seed("configuration/catalog.json", []byte("changed"))
				case "lease":
					f.objects.seed(workerLeaseName, f.lease.record.bytes())
				case "run-cancel":
					stopRun()
				case "origin-cancel":
					stopOrigin()
				}
				return pagerduty.Response{Status: 202}, nil
			}
			got, err := f.coordinator.Run(ctx, f.lease)
			if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || got.ObservedStatus() != 202 || !got.Dispatched() || outcomes != 1 || f.sends.Load() != 1 {
				t.Fatal("observed202 was discarded", err)
			}
		})
	}
}

func TestDiagnosticCoordinatorConfirmsLostReceiptWithoutResend(t *testing.T) {
	f := coordinatorFixture(t)
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.name == diagnosticObjectName && c.method == "REPLACE" {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || f.sends.Load() != 1 || preparationCalls(f.objects, "REPLACE", diagnosticObjectName) != 1 {
		t.Fatal("receipt confirmation failed", err)
	}
}

func TestDiagnosticCoordinatorReportsUnconfirmed202WithoutRebasing(t *testing.T) {
	for _, kind := range []string{"denied", "uncertain", "conflicting-terminal", "deadline", "claim-expiry", "clock-regression"} {
		t.Run(kind, func(t *testing.T) {
			f := coordinatorFixture(t)
			want := "write_failed"
			if kind == "conflicting-terminal" {
				want = "identity_conflict"
			}
			if kind == "deadline" {
				want = "deadline_exceeded"
			}
			if kind == "claim-expiry" {
				want = "stale_claim"
			}
			if kind == "clock-regression" {
				want = "invalid_time"
			}
			f.coordinator.send = func(context.Context, []byte, []byte) (pagerduty.Response, error) {
				f.sends.Add(1)
				switch kind {
				case "deadline":
					at := f.lease.deadline
					f.lease.manager.now = func() time.Time { return at }
				case "claim-expiry":
					_, at := f.lease.record.times()
					f.lease.manager.now = func() time.Time { return at }
				case "clock-regression":
					at, _ := f.lease.record.times()
					f.lease.manager.now = func() time.Time { return at.Add(-time.Second) }
				case "conflicting-terminal":
					r := f.saved(t)
					s := r.Snapshot()
					next, e := r.Finish(DiagnosticClaimRef{s.OwnerToken, s.ClaimToken}, 400, f.lease.manager.now())
					if e != nil {
						t.Fatal(e)
					}
					body, _ := next.Encode()
					f.objects.seed(diagnosticObjectName, body)
				}
				return pagerduty.Response{Status: 202}, nil
			}
			if kind == "denied" || kind == "uncertain" {
				f.objects.before = func(_ context.Context, c discoveryCall) error {
					if c.method == "REPLACE" && c.name == diagnosticObjectName {
						if kind == "denied" {
							return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
						}
						return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
					}
					return nil
				}
				if kind == "uncertain" {
					want = "identity_conflict"
				}
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			diagnosticRunFailure(t, err, want)
			if !got.Dispatched() || got.ObservedStatus() != 202 || got.Record().Snapshot().State != DiagnosticSending || f.sends.Load() != 1 || f.saved(t).Snapshot().State == DiagnosticAccepted {
				t.Fatal("unconfirmed202 reported durable or replayed")
			}
		})
	}
}
