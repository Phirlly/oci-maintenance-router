package ocistorage

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
	Conflict             Code = "conflict"
	Service              Code = "service"
)

// Error contains only controlled diagnostics. WriteUncertain means a write may
// have applied: reread and revalidate before dependent work, never retry blindly.
// Conflict also requires rereading. No error contains a usable object or ETag.
type Error struct {
	Code           Code
	Status         int
	WriteUncertain bool
}

func (e *Error) Error() string { return "object storage: " + string(e.Code) }

// Unwrap exposes only standard cancellation identity, never provider errors.
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

func failure(ctx context.Context, code Code, status int, uncertain bool) *Error {
	switch ctx.Err() {
	case context.Canceled:
		code = Canceled
	case context.DeadlineExceeded:
		code = DeadlineExceeded
	}
	return &Error{Code: code, Status: status, WriteUncertain: uncertain}
}
