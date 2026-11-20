package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/ocinotifications"
)

// SetupConfirmationResult preserves verified channel results when later work
// fails. Only a successful Confirm establishes saved paired confirmation.
type SetupConfirmationResult struct {
	Diagnostic   DiagnosticReceipt
	Operator     OperatorReceipt
	Confirmation SetupConfirmation
}

type SetupConfirmationCoordinator struct {
	diagnostic *DiagnosticReceiptVerifier
	operator   *OperatorReceiptVerifier
	objects    SetupConfirmationObjects
}

func NewSetupConfirmationCoordinator(configBytes, catalogBytes []byte, location configuration.LedgerLocation, release string, objects DiagnosticObjects, channel *ocinotifications.Client) (*SetupConfirmationCoordinator, error) {
	d, err := NewDiagnosticReceiptVerifier(configBytes, catalogBytes, location, release, objects)
	if err != nil {
		return nil, err
	}
	if d.input.mode != "TEST" {
		return nil, &SetupConfirmationError{Code: "intent_required"}
	}
	o, err := NewOperatorReceiptVerifier(configBytes, catalogBytes, location, objects, channel)
	if err != nil {
		return nil, err
	}
	return &SetupConfirmationCoordinator{d, o, objects}, nil
}

func (c *SetupConfirmationCoordinator) Confirm(ctx context.Context, lease *Lease) (SetupConfirmationResult, error) {
	var result SetupConfirmationResult
	if c == nil || c.diagnostic == nil || c.operator == nil || !lease.usable() {
		return result, &SetupConfirmationError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return result, setupConfirmationFailure(ctx, "invalid_request", 0, false)
	}
	if lease.manager.environmentID != c.diagnostic.input.intent.EnvironmentID {
		return result, &SetupConfirmationError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	var err error
	result.Diagnostic, err = c.diagnostic.Verify(ctx, lease)
	if err != nil {
		return result, err
	}
	result.Operator, err = c.operator.Verify(ctx, lease)
	if err != nil {
		return result, err
	}
	guard := func() error {
		if err := c.diagnostic.input.current.check(ctx, lease); err != nil {
			e := discoveryFailureFor(ctx, "confirmation_guard", err)
			return setupConfirmationFailure(ctx, e.Code, e.HTTPStatus, false)
		}
		return nil
	}
	if err = guard(); err != nil {
		return result, err
	}
	proposal, err := newSetupConfirmation(result.Diagnostic, result.Operator, lease.manager.now())
	if err != nil {
		return result, err
	}
	objects := setupConfirmationStorage{c.objects, func() error { return currentReserve(ctx, lease) }, guard}
	s, _ := NewSetupConfirmationStore(objects, proposal.data.EnvironmentID, proposal.data.InstallationID)
	saved, err := s.Create(ctx, proposal)
	if err != nil {
		return result, err
	}
	if err = guard(); err != nil {
		return result, err
	}
	now := lease.manager.now()
	if !leaseWallTime(now) || now.Before(saved.confirmedAt()) {
		return result, &SetupConfirmationError{Code: "invalid_time"}
	}
	result.Confirmation = saved
	return result, nil
}
