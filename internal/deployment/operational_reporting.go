package deployment

import (
	"context"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func (ops operations) processLive(ctx context.Context, lease *reconciliation.Lease, result *OperationalResult) error {
	if ops.reportRetry == nil || ops.reportRefresh == nil || ops.process == nil {
		return operationFailure("operation_configuration")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return operationFailure("deadline")
	}
	beforeContext, stopBefore := context.WithTimeout(ctx, 30*time.Second)
	before, beforeErr := ops.reportRetry(beforeContext, lease)
	beforeCode := ""
	if beforeErr != nil {
		beforeCode = operationCode(beforeContext, beforeErr)
	}
	stopBefore()
	// The processor cannot consume the time reserved for reporting its partial
	// result. Reporting failures never erase independently confirmed PD outcomes.
	processingContext, stopProcessing := context.WithDeadline(ctx, deadline.Add(-60*time.Second))
	saved, processingErr := ops.process(processingContext, lease)
	processingErr = recordProcessing(saved, processingErr, true, result)
	stopProcessing()
	after, afterErr := ops.reportRefresh(ctx, lease, saved)
	latest := after
	if !after.Known {
		latest = before
	}
	result.Reporting = &ReportingOutcome{Known: latest.Known, Issues: latest.Issues, Reviewed: after.Reviewed,
		Remaining: latest.Remaining, Deferred: after.Deferred, Pending: latest.Pending,
		Dispatched: before.Dispatched || after.Dispatched, Accepted: before.Accepted || after.Accepted,
		Code: beforeCode}
	if afterErr != nil {
		result.Reporting.Code = operationCode(ctx, afterErr)
	}
	if processingErr != nil {
		return processingErr
	}
	if result.Reporting.Code != "" {
		return operationFailure(result.Reporting.Code)
	}
	return nil
}
