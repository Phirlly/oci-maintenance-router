package reconciliation

import (
	"bytes"
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorExpiryCannotDiscardConfirmedClaim(t *testing.T) {
	for _, kind := range []string{"clock", "controls", "write"} {
		t.Run(kind, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
			old := f.seed(t, OperatorPublishing, true)
			before, _ := old.Encode()
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.method == "GET" && c.name == operatorRunObjectName {
					switch kind {
					case "clock":
						at := old.Snapshot().StartedAt.Add(-time.Second)
						f.lease.manager.now = func() time.Time { return at }
					case "controls":
						f.objects.seed("configuration/router.json", []byte("changed"))
					}
				}
				return nil
			}
			if kind == "write" {
				f.objects.before = func(_ context.Context, c discoveryCall) error {
					if c.method == "REPLACE" && c.name == operatorRunObjectName {
						return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
					}
					return nil
				}
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			want := map[string]string{"clock": "invalid_time", "controls": "configuration_changed", "write": "write_failed"}[kind]
			operatorRunFailure(t, err, want)
			after, _ := got.Record().Encode()
			saved, _ := f.saved(t).Encode()
			if !bytes.Equal(before, after) || !bytes.Equal(before, saved) || got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 {
				t.Fatal("failed expiry discarded or resent consumed claim")
			}
		})
	}
}
