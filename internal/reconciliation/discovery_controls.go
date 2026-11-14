package reconciliation

import (
	"context"
	"crypto/sha256"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

const discoveryReserve = 10 * time.Second

func (r *discoveryRun) reserve() error {
	if err := r.lease.remaining(r.ctx, discoveryReserve, true); err != nil {
		return discoveryFailureFor(r.ctx, "lease_failed", err)
	}
	return nil
}

// Control reads deliberately do not recursively guard themselves. This check
// coordinates cooperating workers; it is not an atomic configuration fence.
func (r *discoveryRun) guard() error {
	if err := r.reserve(); err != nil {
		return err
	}
	if err := r.lease.Check(r.ctx, discoveryReserve); err != nil {
		return discoveryFailureFor(r.ctx, "lease_failed", err)
	}
	for _, control := range []struct {
		name string
		hash [32]byte
	}{
		{"configuration/router.json", r.owner.configHash},
		{"configuration/catalog.json", r.owner.catalogHash},
	} {
		if err := r.reserve(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
		object, err := r.owner.objects.Get(ctx, control.name)
		late := ctx.Err()
		cancel()
		if err != nil || late != nil {
			// This is a raw injected backend boundary. It cannot supply trusted
			// coordinator errors or override the controlled failure category.
			failure := discoveryFailureFor(r.ctx, "configuration_read", nil)
			if detail, ok := err.(*ocistorage.Error); ok && detail != nil && detail.Status >= 100 && detail.Status <= 599 {
				failure.HTTPStatus = detail.Status
			}
			return failure
		}
		if len(object.Body) > 1<<20 || sha256.Sum256(object.Body) != control.hash {
			return &DiscoveryError{Code: "configuration_changed"}
		}
	}
	return r.reserve()
}
