package deployment

import (
	"context"
	"errors"
	"regexp"

	"oci-maintenance-router/internal/reconciliation"
)

type operationFailure string

func (e operationFailure) Error() string { return string(e) }

var controlledCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Only owned coordinator diagnostics are projected; provider errors stay private.
func operationCode(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "deadline_exceeded"
		}
		return "canceled"
	}
	code := "operation_failed"
	switch e := err.(type) {
	case operationFailure:
		code = string(e)
	case *reconciliation.PreparationError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.DiagnosticRunError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.OperatorRunError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.SetupConfirmationError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.LiveSelectionError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.ProcessingError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.ReportingError:
		if e != nil {
			code = e.Code
		}
	case *reconciliation.RecoveryReviewError:
		if e != nil {
			code = e.Code
		}
	}
	if !controlledCode.MatchString(code) {
		return "operation_failed"
	}
	return code
}
