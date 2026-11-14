package reconciliation

import "context"

// PreparationError contains controlled diagnostics, never source/state contents.
type PreparationError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *PreparationError) Error() string { return "preparation: " + e.Code }
func (e *PreparationError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

// Call only for errors returned by owned domain/store/control implementations.
// The raw history backend uses its separate fixed-category handler.
func preparationFailure(ctx context.Context, code string, err error) *PreparationError {
	r := &PreparationError{Code: code}
	switch e := err.(type) {
	case *PreparationError:
		if e != nil {
			*r = *e
		}
	case *DiscoveryError:
		if e != nil {
			r.Code, r.HTTPStatus = e.Code, e.HTTPStatus
		}
	case *LeaseError:
		if e != nil {
			r.Code, r.HTTPStatus = e.Code, e.HTTPStatus
		}
	case *BaselineStorageError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *BaselineError:
		if e != nil {
			r.Code = e.Code
		}
	case *Error:
		if e != nil {
			r.Code = string(e.Code)
		}
	}
	if r.HTTPStatus < 100 || r.HTTPStatus > 599 {
		r.HTTPStatus = 0
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			r.Code = "canceled"
		case context.DeadlineExceeded:
			r.Code = "deadline_exceeded"
		}
	}
	return r
}
