package ociownership

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
)

// Error exposes controlled categories and HTTP status, never source contents,
// headers or provider errors. A 404 is not proof that a resource is absent.
type Error struct {
	Code   Code
	Status int
}

func (e *Error) Error() string { return "resource metadata read: " + string(e.Code) }
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
func failure(ctx context.Context, code Code, status int) *Error {
	switch ctx.Err() {
	case context.Canceled:
		code = Canceled
	case context.DeadlineExceeded:
		code = DeadlineExceeded
	}
	return &Error{Code: code, Status: status}
}
