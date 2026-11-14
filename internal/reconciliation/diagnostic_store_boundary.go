package reconciliation

import (
	"context"
	"time"
)

// DiagnosticStorageError exposes only controlled diagnostics, never backend text.
type DiagnosticStorageError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *DiagnosticStorageError) Error() string { return "diagnostic storage: " + e.Code }
func (e *DiagnosticStorageError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}
func diagnosticStoreContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, &DiagnosticStorageError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return nil, nil, diagnosticStorageFailure(ctx, "canceled", 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &DiagnosticStorageError{Code: "deadline_required"}
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	return bounded, cancel, nil
}
func diagnosticStorageFailure(ctx context.Context, code string, status int, uncertain bool) *DiagnosticStorageError {
	switch ctx.Err() {
	case context.Canceled:
		code = "canceled"
	case context.DeadlineExceeded:
		code = "deadline_exceeded"
	}
	return &DiagnosticStorageError{code, status, uncertain}
}
