package reconciliation

import (
	"context"
	"crypto/sha256"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

const discoveryReserve = 10 * time.Second

type currentControls struct {
	objects                 currentObjects
	configHash, catalogHash [32]byte
}

type currentObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
}

func (d *Discovery) controls() currentControls {
	return currentControls{d.objects, d.configHash, d.catalogHash}
}
func currentReserve(ctx context.Context, lease *Lease) error {
	if err := lease.remaining(ctx, discoveryReserve, true); err != nil {
		return discoveryFailureFor(ctx, "lease_failed", err)
	}
	return nil
}

// Control reads do not recursively guard themselves. These individual checks
// coordinate cooperating workers; they are not an atomic configuration fence.
func (c currentControls) check(ctx context.Context, lease *Lease) error {
	if err := currentReserve(ctx, lease); err != nil {
		return err
	}
	if err := lease.Check(ctx, discoveryReserve); err != nil {
		return discoveryFailureFor(ctx, "lease_failed", err)
	}
	for _, control := range []struct {
		name string
		hash [32]byte
	}{{"configuration/router.json", c.configHash}, {"configuration/catalog.json", c.catalogHash}} {
		if err := currentReserve(ctx, lease); err != nil {
			return err
		}
		readCtx, cancel := context.WithTimeout(ctx, discoveryReserve)
		object, err := c.objects.Get(readCtx, control.name)
		late := readCtx.Err()
		cancel()
		if err != nil || late != nil {
			// Raw adapters cannot forge trusted coordinator diagnostics.
			failure := discoveryFailureFor(ctx, "configuration_read", nil)
			failure.HTTPStatus = baselineStorageStatus(err)
			return failure
		}
		if len(object.Body) > 1<<20 || sha256.Sum256(object.Body) != control.hash {
			return &DiscoveryError{Code: "configuration_changed"}
		}
	}
	return currentReserve(ctx, lease)
}
