package deployment

import (
	"context"
	"errors"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

// This entrypoint check also covers no-work and retained diagnostic returns.
// Like coordinator checks, it is cooperative, not an atomic configuration fence.
func verifyCurrent(ctx context.Context, lease *reconciliation.Lease, store Store, s Settings) error {
	if err := lease.Check(ctx, 10*time.Second); err != nil {
		return &ownershipGuardError{cause: err}
	}
	for _, expected := range []struct{ name, hash string }{
		{"configuration/router.json", s.ConfigurationSHA256},
		{"configuration/catalog.json", s.CatalogSHA256},
	} {
		read, cancel := context.WithTimeout(ctx, 10*time.Second)
		obj, err := store.Get(read, expected.name)
		late := read.Err()
		cancel()
		if err != nil || late != nil {
			return errors.New("current_configuration_read")
		}
		if len(obj.Body) > 1<<20 || digest(obj.Body) != expected.hash {
			return errors.New("configuration_changed")
		}
	}
	if err := lease.Check(ctx, 10*time.Second); err != nil {
		return &ownershipGuardError{cause: err}
	}
	return nil
}
