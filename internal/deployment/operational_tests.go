package deployment

import (
	"context"

	"oci-maintenance-router/internal/reconciliation"
)

func (ops operations) test(ctx context.Context, lease *reconciliation.Lease, result *OperationalResult) error {
	diagnostic, err := ops.diagnostic(ctx, lease)
	saved := diagnostic.Record().Snapshot()
	result.Diagnostic = &TestOutcome{State: string(saved.State), Dispatched: diagnostic.Dispatched(), ObservedStatus: diagnostic.ObservedStatus()}
	if controlledCode.MatchString(saved.Reason) {
		result.Diagnostic.Code = saved.Reason
	}
	if err != nil {
		result.Diagnostic.Code = operationCode(ctx, err)
		return err
	}
	if saved.State != reconciliation.DiagnosticAccepted {
		return operationFailure("diagnostic_not_accepted")
	}
	if ops.operator == nil {
		return nil
	}
	operator, err := ops.operator(ctx, lease)
	channel := operator.Record().Snapshot()
	result.Operator = &TestOutcome{State: string(channel.State), Dispatched: operator.Dispatched(), ObservedStatus: operator.ObservedStatus()}
	if controlledCode.MatchString(channel.Reason) {
		result.Operator.Code = channel.Reason
	}
	if err != nil {
		result.Operator.Code = operationCode(ctx, err)
		return err
	}
	if channel.State != reconciliation.OperatorAccepted {
		return operationFailure("operator_not_accepted")
	}
	return nil
}
