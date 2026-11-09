package pagerduty

import "context"

type Code string

const (
	InvalidConfiguration Code = "invalid_configuration"
	InvalidRequest       Code = "invalid_request"
	DeadlineRequired     Code = "deadline_required"
	Canceled             Code = "canceled"
	DeadlineExceeded     Code = "deadline_exceeded"
	Transport            Code = "transport"
	TooLarge             Code = "too_large"
)

// Error excludes request/response bodies, keys, URLs and underlying errors.
// MayHaveSent is conservative after dispatch; false proves no dispatch occurred.
type Error struct {
	Code        Code
	MayHaveSent bool
}

func (e *Error) Error() string { return "pagerduty request: " + string(e.Code) }
func (e *Error) Unwrap() error {
	switch e.Code {
	case Canceled:
		return context.Canceled
	case DeadlineExceeded:
		return context.DeadlineExceeded
	default:
		return nil
	}
}
func failure(ctx context.Context, sent bool) *Error {
	code := Transport
	switch ctx.Err() {
	case context.Canceled:
		code = Canceled
	case context.DeadlineExceeded:
		code = DeadlineExceeded
	}
	return &Error{Code: code, MayHaveSent: sent}
}
