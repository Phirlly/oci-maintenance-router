package reconciliation

import (
	"context"
	"time"
)

// OperatorChallengeStorageError exposes only controlled diagnostics, never backend text.
type OperatorChallengeStorageError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *OperatorChallengeStorageError) Error() string {
	return "operator challenge storage: " + e.Code
}
func (e *OperatorChallengeStorageError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}
func operatorStoreContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, &OperatorChallengeStorageError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return nil, nil, operatorStorageFailure(ctx, "canceled", 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &OperatorChallengeStorageError{Code: "deadline_required"}
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	return bounded, cancel, nil
}
func operatorStorageFailure(ctx context.Context, code string, status int, uncertain bool) *OperatorChallengeStorageError {
	switch ctx.Err() {
	case context.Canceled:
		code = "canceled"
	case context.DeadlineExceeded:
		code = "deadline_exceeded"
	}
	return &OperatorChallengeStorageError{code, status, uncertain}
}
