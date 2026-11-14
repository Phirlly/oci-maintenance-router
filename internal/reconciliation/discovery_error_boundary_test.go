package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiscoveryCurrentReadCannotForgeCoordinatorDiagnostics(t *testing.T) {
	for _, external := range []error{
		&DiscoveryError{Code: "private provider detail"},
		&LeaseError{Code: "private provider detail"},
		&Error{Code: "private provider detail"},
		fmt.Errorf("backend: %w", &DiscoveryError{Code: "private provider detail"}),
		errors.Join(errors.New("private provider detail"), &LeaseError{Code: "ownership_lost"}),
		&DiscoveryError{Code: "canceled"},
	} {
		d, s, lease := discoveryFixture(t, nil, nil)
		s.before = func(_ context.Context, call discoveryCall) error {
			if call.name == "configuration/router.json" {
				return external
			}
			return nil
		}
		got, err := d.Run(leaseBudget(t), lease)
		discoveryFailure(t, got, err, "configuration_read")
		if err.Error() != "discovery: configuration_read" {
			t.Fatal("raw backend diagnostics escaped")
		}
	}
}

func TestDiscoveryCurrentReadRetainsOnlyValidDirectStorageStatus(t *testing.T) {
	for _, status := range []int{-1, 99, 403, 503, 600} {
		d, s, lease := discoveryFixture(t, nil, nil)
		s.before = func(_ context.Context, call discoveryCall) error {
			if call.name == "configuration/catalog.json" {
				return &ocistorage.Error{Code: "private provider detail", Status: status}
			}
			return nil
		}
		got, err := d.Run(leaseBudget(t), lease)
		discoveryFailure(t, got, err, "configuration_read")
		want := status
		if want < 100 || want > 599 {
			want = 0
		}
		if err.(*DiscoveryError).HTTPStatus != want {
			t.Fatal("wrong bounded status")
		}
	}
}
