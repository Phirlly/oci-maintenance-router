package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

type setupReadiness struct {
	input   setupControls
	objects setupCurrentObjects
	read    func(context.Context) (OperatorChannel, error)
}

func newSetupReadiness(configBytes, catalogBytes []byte, location configuration.LedgerLocation, objects setupCurrentObjects, client *ocinotifications.Client) (*setupReadiness, error) {
	if client == nil {
		return nil, &SetupError{Code: "invalid_request"}
	}
	c, err := readSetupControls(configBytes, catalogBytes, location, objects)
	if err != nil {
		return nil, err
	}
	if c.config.Mode != "LIVE" || c.config.Setup.Action != "NONE" {
		return nil, &SetupError{Code: "intent_required"}
	}
	return &setupReadiness{c, objects, operatorChannelReader(client, c.desired)}, nil
}

func (v *setupReadiness) verify(ctx context.Context, lease *Lease, release string) error {
	if !leaseDigest.MatchString(release) {
		return &SetupError{Code: "invalid_request"}
	}
	_, err := v.check(ctx, lease, release)
	return err
}

func (v *setupReadiness) verifyOperator(ctx context.Context, lease *Lease) (OperatorReceipt, error) {
	return v.check(ctx, lease, "")
}

// An empty release selects operator-only verification. It cannot authorize
// maintenance delivery and deliberately does not read diagnostic state.
func (v *setupReadiness) check(ctx context.Context, lease *Lease, release string) (OperatorReceipt, error) {
	if v == nil || v.objects == nil || v.read == nil || !lease.usable() {
		return OperatorReceipt{}, &SetupError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return OperatorReceipt{}, setupFailure(ctx, "invalid_request", 0, false)
	}
	if lease.manager.environmentID != v.input.environment {
		return OperatorReceipt{}, &SetupError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	r := &setupReadinessRun{owner: v, ctx: ctx, lease: lease}
	objects := setupCurrentStorage{v.objects, func() error { return currentReserve(ctx, lease) }, r.guard}
	r.store, _ = newSetupCurrentStore(objects, v.input.environment, v.input.config.InstallationID)
	return r.verify(release)
}
