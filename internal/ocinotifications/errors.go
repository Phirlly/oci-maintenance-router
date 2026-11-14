package ocinotifications

import "context"

type Code string

const (
	InvalidConfiguration Code = "invalid_configuration"
	InvalidRequest       Code = "invalid_request"
	DeadlineRequired     Code = "deadline_required"
	Canceled             Code = "canceled"
	DeadlineExceeded     Code = "deadline_exceeded"
	Authentication       Code = "authentication"
	Transport            Code = "transport"
	InvalidResponse      Code = "invalid_response"
	TooLarge             Code = "too_large"
	Service              Code = "service"
	InactiveChannel      Code = "inactive_channel"
)

// Error excludes targets, URLs, bodies and provider details. MayHaveSent is
// conservative after publish dispatch; no error grants retry permission.
type Error struct {
	Code        Code
	Status      int
	MayHaveSent bool
}

func (e *Error) Error() string { return "operator notification: " + string(e.Code) }
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
func failure(ctx context.Context, code Code, status int, sent bool) *Error {
	switch ctx.Err() {
	case context.Canceled:
		code = Canceled
	case context.DeadlineExceeded:
		code = DeadlineExceeded
	}
	return &Error{Code: code, Status: status, MayHaveSent: sent}
}
