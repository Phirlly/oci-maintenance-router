package reconciliation

import "context"

// SetupConfirmationError contains only controlled diagnostics, no object/provider data.
type SetupConfirmationError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *SetupConfirmationError) Error() string { return "setup confirmation: " + e.Code }
func (e *SetupConfirmationError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

func setupConfirmationFailure(ctx context.Context, code string, status int, uncertain bool) *SetupConfirmationError {
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			code = "canceled"
		case context.DeadlineExceeded:
			code = "deadline_exceeded"
		}
	}
	return &SetupConfirmationError{code, status, uncertain}
}
