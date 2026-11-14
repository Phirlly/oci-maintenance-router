package deployment

import (
	"context"

	"oci-maintenance-router/internal/reconciliation"
)

func (ops operations) processWork(ctx context.Context, lease *reconciliation.Lease, live bool, result *OperationalResult) error {
	saved, err := ops.process(ctx, lease)
	result.Processing = &ProcessingOutcome{Prepared: saved.Prepared, Accepted: saved.Accepted,
		Deferred: saved.Deferred, Suppressed: saved.Suppressed, Issues: len(saved.Issues), Attempts: len(saved.Attempts)}
	if r := saved.Recovery; r != nil {
		result.Recovery = &RecoveryOutcome{RequestID: r.RequestID, Status: r.Status, Reason: r.Reason, InvalidationReason: r.InvalidationReason, Ambiguous: r.Ambiguous}
	}
	if err != nil {
		return err
	}
	if !saved.Completed {
		return operationFailure("processing_incomplete")
	}
	result.ScanCompleted = live
	result.Status = "recovery-recorded"
	if live {
		result.Status = "reconciled"
	}
	return nil
}
