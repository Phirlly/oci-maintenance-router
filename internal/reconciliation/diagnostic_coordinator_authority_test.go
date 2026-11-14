package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticCoordinatorStopsChangedAuthorityBeforeSend(t *testing.T) {
	for _, stage := range []string{"initial", "vault", "claim", "claim-confirm"} {
		for _, control := range []string{"config", "catalog", "lease"} {
			t.Run(stage+"/"+control, func(t *testing.T) {
				f := coordinatorFixture(t)
				changed := false
				change := func() {
					changed = true
					switch control {
					case "config":
						f.objects.seed("configuration/router.json", []byte("changed"))
					case "catalog":
						f.objects.seed("configuration/catalog.json", []byte("changed"))
					case "lease":
						f.objects.seed(workerLeaseName, f.lease.record.bytes())
					}
				}
				switch stage {
				case "initial":
					change()
				case "vault":
					old := f.coordinator.current
					f.coordinator.current = func(c context.Context, id string) (diagnosticCredential, error) {
						b, e := old(c, id)
						change()
						return b, e
					}
				default:
					pending := false
					f.objects.after = func(_ context.Context, c discoveryCall) error {
						if c.name != diagnosticObjectName {
							return nil
						}
						if c.method == "CREATE" {
							if stage == "claim-confirm" {
								pending = true
								return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
							}
							change()
						} else if pending && c.method == "GET" {
							pending = false
							change()
						}
						return nil
					}
				}
				got, err := f.coordinator.Run(leaseBudget(t), f.lease)
				want := "configuration_changed"
				if control == "lease" {
					want = "ownership_lost"
				}
				diagnosticRunFailure(t, err, want)
				if !changed || got.Dispatched() || f.sends.Load() != 0 {
					t.Fatal("changed authority still dispatched")
				}
				if stage == "claim" || stage == "claim-confirm" {
					if got.Record().Snapshot().State != DiagnosticSending || f.saved(t).Snapshot().State != DiagnosticSending {
						t.Fatal("consumed unsent claim discarded")
					}
				}
			})
		}
	}
}

func TestDiagnosticCoordinatorCanceledClaimCannotDispatchOnRepeat(t *testing.T) {
	f := coordinatorFixture(t)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.objects.after = func(c context.Context, call discoveryCall) error {
		if call.name == diagnosticObjectName && call.method == "CREATE" {
			cancel()
			return c.Err()
		}
		return nil
	}
	got, err := f.coordinator.Run(ctx, f.lease)
	diagnosticRunFailure(t, err, "canceled")
	if got.Dispatched() || f.sends.Load() != 0 {
		t.Fatal("canceled claim dispatched")
	}
	f.objects.after = nil
	again, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || again.Record().Snapshot().State != DiagnosticSending || again.Dispatched() || f.reads.Load() != 1 || f.sends.Load() != 0 {
		t.Fatal("restarted same lease resumed consumed claim", err)
	}
}
