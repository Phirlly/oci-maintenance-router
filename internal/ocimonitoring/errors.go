package ocimonitoring

import "context"

// Error contains only controlled classification, never targets or provider data.
type Error struct{ Code string }

func (e *Error) Error() string { return "operational metrics: " + e.Code }
func (e *Error) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func failure(ctx context.Context, code string) error {
	switch ctx.Err() {
	case context.Canceled:
		code = "canceled"
	case context.DeadlineExceeded:
		code = "deadline_exceeded"
	}
	return &Error{Code: code}
}
