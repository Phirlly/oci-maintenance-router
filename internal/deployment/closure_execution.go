package deployment

import (
	"context"
	"regexp"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/reconciliation"
)

var closureRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func matchesClosure(c configuration.Config, request command) bool {
	q := c.Recovery
	return (c.SchemaVersion == 7 || c.SchemaVersion == 8 || c.SchemaVersion == 9) && c.Mode == "PAUSED" && q != nil &&
		(q.Kind == "CLOSE_BATCH" || q.Kind == "CLOSE_REVISION") && q.RequestID == request.recoveryRequestID
}

// Handler has already matched the pinned PAUSED intent before acquiring a lease.
func (ops operations) closeWork(ctx context.Context, lease *reconciliation.Lease, request command, result *OperationalResult) error {
	if err := ops.processWork(ctx, lease, false, result); err != nil {
		return err
	}
	r, p := result.Recovery, result.Processing
	if r == nil || r.RequestID != request.recoveryRequestID || r.Status != "CLOSED" || r.Reason != "" || r.InvalidationReason != "" || r.Ambiguous || p == nil || *p != (ProcessingOutcome{}) {
		return operationFailure("closure_not_completed")
	}
	result.Status = "closure-completed"
	return nil
}
