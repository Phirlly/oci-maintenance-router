package deployment

import (
	"context"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

type operationalObjects interface {
	Store
	reconciliation.PreparationObjects
}

func (h *Handler) runOperational(original context.Context, started time.Time, s Settings, store Store, p pinnedInputs, request command) (result OperationalResult) {
	result = OperationalResult{ResponseVersion: 2, Status: "not-ready", Operation: request.operation,
		Mode: p.config.Mode, CheckAttemptID: request.attemptID, ConfigurationSHA256: s.ConfigurationSHA256,
		ReleaseSHA256: s.ReleaseSHA256, RoutingGeneration: p.generation}
	originalDeadline, _ := original.Deadline()
	deadline := originalDeadline.Add(-10 * time.Second)
	if cap := started.Add(240 * time.Second); cap.Before(deadline) {
		deadline = cap
	}
	ctx, cancel := context.WithDeadline(original, deadline)
	defer cancel()
	if ctx.Err() != nil {
		result.Code = "deadline"
		return result
	}
	objects, ok := store.(operationalObjects)
	if !ok {
		result.Code = "storage_configuration"
		return result
	}
	manager, err := reconciliation.NewLeaseManager(objects, p.environment)
	if err != nil {
		result.Code = "worker_configuration"
		return result
	}
	lease, err := manager.Acquire(ctx)
	if err != nil {
		result.Code = "ownership_failed"
		return result
	}
	if lease == nil {
		result.Status = "coalesced"
		return result
	}
	defer func() {
		cleanupDeadline := time.Now().Add(10 * time.Second)
		if originalDeadline.Before(cleanupDeadline) {
			cleanupDeadline = originalDeadline
		}
		cleanup, done := context.WithDeadline(context.WithoutCancel(original), cleanupDeadline)
		defer done()
		if lease.Release(cleanup) != nil {
			result.Status, result.CleanupCode, result.ScanCompleted = "not-ready", "release_failed", false
			result.Review = nil
			result.SavedNotices = nil
			result.ClosureReview = nil
		}
	}()
	if err := verifyCurrent(ctx, lease, store, s); err != nil {
		result.Code = err.Error() // Only locally constructed controlled errors.
		return result
	}
	ops, err := h.compose(s, store, p, request.operation)
	if err != nil {
		result.Code = "operation_configuration"
		return result
	}
	if err := ops.execute(ctx, lease, p.config, request, &result); err != nil {
		result.Status, result.Code, result.ScanCompleted = "not-ready", operationCode(ctx, err), false
		return result
	}
	if err := verifyCurrent(ctx, lease, store, s); err != nil {
		result.Status, result.Code, result.ScanCompleted = "not-ready", err.Error(), false
		result.Review = nil
		result.SavedNotices = nil
		result.ClosureReview = nil
	}
	return result
}
