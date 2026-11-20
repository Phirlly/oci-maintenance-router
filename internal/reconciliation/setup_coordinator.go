package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocisecrets"
)

type SetupResult struct {
	Diagnostic   DiagnosticResult
	Operator     OperatorResult
	Confirmation SetupConfirmationResult
}

type SetupCoordinator struct {
	input      setupControls
	release    string
	objects    setupCurrentObjects
	channel    *ocinotifications.Client
	diagnostic *DiagnosticCoordinator
	operator   *OperatorCoordinator
	read       func(context.Context) (OperatorChannel, error)
}

func NewSetupCoordinator(configBytes, catalogBytes []byte, location configuration.LedgerLocation, release string, objects PreparationObjects, channel *ocinotifications.Client, vault *ocisecrets.Client) (*SetupCoordinator, error) {
	if !leaseDigest.MatchString(release) || channel == nil {
		return nil, &SetupError{Code: "invalid_request"}
	}
	input, err := readSetupControls(configBytes, catalogBytes, location, objects)
	if err != nil {
		return nil, err
	}
	if input.config.Mode != "TEST" || input.config.Setup.Action == "NONE" {
		return nil, &SetupError{Code: "intent_required"}
	}
	c := &SetupCoordinator{input: input, release: release, objects: objects, channel: channel}
	c.read = operatorChannelReader(channel, input.desired)
	if input.config.Setup.Action == "SEND_TEST" {
		d, o := input.requested(release)
		c.diagnostic, err = newDiagnosticCoordinator(input.diagnosticInput(d, ""), objects, vault)
		if err != nil {
			return nil, err
		}
		c.operator, err = newOperatorCoordinator(input.operatorInput(o, ""), objects, channel)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *SetupCoordinator) Run(ctx context.Context, lease *Lease) (SetupResult, error) {
	if c == nil || c.input.config.Setup == nil || c.objects == nil || c.read == nil || !lease.usable() {
		return SetupResult{}, &SetupError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return SetupResult{}, setupFailure(ctx, "invalid_request", 0, false)
	}
	if lease.manager.environmentID != c.input.environment {
		return SetupResult{}, &SetupError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	r := &setupRun{owner: c, ctx: ctx, lease: lease}
	objects := setupCurrentStorage{c.objects, func() error { return currentReserve(ctx, lease) }, r.guard}
	r.store, _ = newSetupCurrentStore(objects, c.input.environment, c.input.config.InstallationID)
	var err error
	if err = r.guard(); err == nil {
		if c.input.config.Setup.Action == "SEND_TEST" {
			err = r.send()
		} else {
			err = r.confirm()
		}
	}
	if err == nil {
		err = r.guard()
	}
	return r.result, err
}
