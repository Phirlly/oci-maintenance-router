package reconciliation

import (
	"context"
	"errors"

	"oci-maintenance-router/internal/ocistorage"
)

// LeaseError excludes tokens, object bytes and provider diagnostics.
type LeaseError struct {
	Code       string
	HTTPStatus int
}

func (e *LeaseError) Error() string { return "lease: " + e.Code }
func (e *LeaseError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}
func leaseError(ctx context.Context, code string, err error) *LeaseError {
	result := &LeaseError{Code: code}
	var detail *ocistorage.Error
	if errors.As(err, &detail) && detail != nil && detail.Status >= 100 && detail.Status <= 599 {
		result.HTTPStatus = detail.Status
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			result.Code = "canceled"
		case context.DeadlineExceeded:
			result.Code = "deadline_exceeded"
		}
	}
	return result
}
func leaseConflict(err error) bool {
	var e *LeaseError
	return errors.As(err, &e) && e != nil && e.Code == "conflict"
}
func leaseRequest(ctx context.Context) error {
	if ctx == nil {
		return &LeaseError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return leaseError(ctx, "canceled", nil)
	}
	if _, ok := ctx.Deadline(); !ok {
		return &LeaseError{Code: "deadline_required"}
	}
	return nil
}
