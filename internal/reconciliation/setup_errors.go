package reconciliation

import "context"

// SetupError contains owned diagnostics, never provider or customer content.
type SetupError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *SetupError) Error() string { return "setup: " + e.Code }
func (e *SetupError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

func setupFailure(ctx context.Context, code string, status int, uncertain bool) *SetupError {
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			code = "canceled"
		case context.DeadlineExceeded:
			code = "deadline_exceeded"
		}
	}
	return &SetupError{code, status, uncertain}
}
