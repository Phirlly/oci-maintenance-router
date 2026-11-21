package deployment

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/reconciliation"
)

func liveProcessingOperation(operation string) bool {
	return operation == "reconcile" || operation == "recover-work"
}

func matchesSendingRecovery(c configuration.Config, request command) bool {
	q := c.Recovery
	return (c.SchemaVersion == 7 || c.SchemaVersion == 8 || c.SchemaVersion == 9) && c.Mode == "LIVE" && q != nil &&
		(q.Kind == "RETRY_BATCH" || q.Kind == "APPROVE_REVISION") && q.RequestID == request.recoveryRequestID
}

func (ops operations) recoverWork(ctx context.Context, lease *reconciliation.Lease, c configuration.Config, request command, result *OperationalResult) error {
	if err := ops.processLive(ctx, lease, result); err != nil {
		return err
	}
	want := "API_ACCEPTED"
	if c.Recovery.Kind == "APPROVE_REVISION" {
		want = "APPROVED"
	}
	r := result.Recovery
	if r == nil || r.RequestID != request.recoveryRequestID || r.Status != want || r.Reason != "" || r.InvalidationReason != "" {
		return operationFailure("recovery_not_completed")
	}
	// Earlier accepted retry uncertainty remains historical evidence. Approval
	// confirms only the recorded batches, not their downstream delivery.
	result.Status = "recovery-verified"
	return nil
}
