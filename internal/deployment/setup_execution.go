package deployment

import (
	"context"
	"oci-maintenance-router/internal/reconciliation"
)

func setupOperation(c *reconciliation.SetupCoordinator, confirm bool) func(context.Context, *reconciliation.Lease, *OperationalResult) error {
	return func(ctx context.Context, lease *reconciliation.Lease, result *OperationalResult) error {
		saved, err := c.Run(ctx, lease)
		if confirm {
			paired := saved.Confirmation
			if err = pairedReceiptOutcome(ctx, string(paired.Diagnostic.Record().Snapshot().State), string(paired.Operator.Record().Snapshot().State), err, result); err != nil {
				return err
			}
			if paired.Confirmation.ID() == "" {
				return operationFailure("confirmation_not_saved")
			}
			return nil
		}
		return setupSendOutcome(ctx, saved, err, result)
	}
}

// Preserve partial dispatch/acceptance when a subsequent channel or guard fails.
// Only the coordinator's successful return establishes the completed operation.
func setupSendOutcome(ctx context.Context, saved reconciliation.SetupResult, err error, result *OperationalResult) error {
	d := saved.Diagnostic.Record().Snapshot()
	result.Diagnostic = &TestOutcome{State: string(d.State), Dispatched: saved.Diagnostic.Dispatched(), ObservedStatus: saved.Diagnostic.ObservedStatus()}
	if controlledCode.MatchString(d.Reason) {
		result.Diagnostic.Code = d.Reason
	}
	if d.State != reconciliation.DiagnosticAccepted {
		if err == nil {
			err = operationFailure("diagnostic_not_accepted")
		}
		result.Diagnostic.Code = operationCode(ctx, err)
		return err
	}
	o := saved.Operator.Record().Snapshot()
	result.Operator = &TestOutcome{State: string(o.State), Dispatched: saved.Operator.Dispatched(), ObservedStatus: saved.Operator.ObservedStatus()}
	if controlledCode.MatchString(o.Reason) {
		result.Operator.Code = o.Reason
	}
	if err != nil {
		result.Operator.Code = operationCode(ctx, err)
		return err
	}
	if o.State != reconciliation.OperatorAccepted {
		return operationFailure("operator_not_accepted")
	}
	return nil
}
