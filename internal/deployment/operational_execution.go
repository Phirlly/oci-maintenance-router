package deployment

import (
	"context"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/reconciliation"
)

func (ops operations) execute(ctx context.Context, lease *reconciliation.Lease, c configuration.Config, request command, result *OperationalResult) error {
	operation := request.operation
	if operation == "list-notices" {
		page, err := ops.list(ctx, request)
		if err != nil {
			return err
		}
		result.SavedNotices, result.Status = page, "listed"
		return nil
	}
	if operation == "review-notice" {
		page, err := ops.review(ctx, request)
		if err != nil {
			return err
		}
		result.Review, result.Status = page, "reviewed"
		return nil
	}
	if operation == "reconcile" && c.Mode != "LIVE" && c.Recovery != nil {
		if err := ops.processWork(ctx, lease, false, result); err != nil {
			return err
		}
	}
	switch c.Mode {
	case "PREVIEW":
		if operation == "deployment-check" && c.Preparation.Initialize {
			if err := ops.initialize(ctx, lease); err != nil {
				return err
			}
		}
		if _, err := ops.prepare(ctx, lease); err != nil {
			return err
		}
		result.BaselinePrepared, result.Status = true, "prepared"
	case "PAUSED":
		if result.Recovery == nil {
			result.Status = "idle"
		}
	case "TEST":
		if operation == "deployment-check" {
			if err := ops.test(ctx, lease, result); err != nil {
				return err
			}
		}
		if result.Recovery == nil {
			result.Status = "idle"
		}
	case "LIVE":
		if operation == "deployment-check" {
			if _, err := ops.selectLive(ctx, lease); err != nil {
				return err
			}
			result.BaselineSelected = true
		} else if err := ops.processWork(ctx, lease, true, result); err != nil {
			return err
		}
	default:
		return operationFailure("operation_unavailable")
	}
	if operation == "deployment-check" {
		result.Status = "operational-verified"
	}
	return nil
}
