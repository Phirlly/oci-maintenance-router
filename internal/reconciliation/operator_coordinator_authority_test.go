package reconciliation

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorStopsChangedAuthorityBeforePublication(t *testing.T) {
	for _, stage := range []string{"initial", "first-channel", "claim", "claim-confirm", "second-channel"} {
		for _, control := range []string{"config", "catalog", "lease"} {
			t.Run(stage+"/"+control, func(t *testing.T) {
				f := operatorCoordinatorFixtureFor(t)
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
				case "first-channel", "second-channel":
					old := f.coordinator.read
					f.coordinator.read = func(c context.Context) (operatorObservation, error) {
						o, e := old(c)
						if (stage == "first-channel" && f.reads.Load() == 1) || (stage == "second-channel" && f.reads.Load() == 2) {
							change()
						}
						return o, e
					}
				default:
					pending := false
					f.objects.after = func(_ context.Context, c discoveryCall) error {
						if c.name != operatorRunObjectName {
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
				operatorRunFailure(t, err, want)
				if !changed || got.Dispatched() || f.sends.Load() != 0 {
					t.Fatal("changed authority still dispatched")
				}
				if stage == "claim" || stage == "claim-confirm" || stage == "second-channel" {
					if got.Record().Snapshot().State != OperatorPublishing || f.saved(t).Snapshot().State != OperatorPublishing {
						t.Fatal("consumed unsent claim discarded")
					}
				}
			})
		}
	}
}

func TestOperatorCoordinatorCanceledClaimCannotDispatchOnRepeat(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.objects.after = func(c context.Context, call discoveryCall) error {
		if call.name == operatorRunObjectName && call.method == "CREATE" {
			cancel()
			return c.Err()
		}
		return nil
	}
	got, err := f.coordinator.Run(ctx, f.lease)
	operatorRunFailure(t, err, "canceled")
	if got.Dispatched() || f.sends.Load() != 0 {
		t.Fatal("canceled claim dispatched")
	}
	f.objects.after = nil
	again, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || again.Record().Snapshot().State != OperatorPublishing || again.Dispatched() || f.reads.Load() != 1 || f.sends.Load() != 0 {
		t.Fatal("same lease resumed consumed claim", err)
	}
}
