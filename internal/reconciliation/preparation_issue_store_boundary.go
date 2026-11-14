package reconciliation

import (
	"context"
	"time"
)

type PreparationIssueStorageError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *PreparationIssueStorageError) Error() string { return "preparation issue storage: " + e.Code }
func (e *PreparationIssueStorageError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}
func preparationIssueContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, &PreparationIssueStorageError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return nil, nil, preparationIssueFailure(ctx, "read_failed", 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &PreparationIssueStorageError{Code: "deadline_required"}
	}
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	return work, cancel, nil
}
func preparationIssueFailure(ctx context.Context, code string, status int, uncertain bool) *PreparationIssueStorageError {
	switch ctx.Err() {
	case context.Canceled:
		code = "canceled"
	case context.DeadlineExceeded:
		code = "deadline_exceeded"
	}
	return &PreparationIssueStorageError{code, status, uncertain}
}
