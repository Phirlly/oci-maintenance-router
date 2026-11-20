package reconciliation

import "context"

// LiveSelectionError contains controlled diagnostics, never source, contact,
// credential or provider content. Selection failure never returns a baseline.
type LiveSelectionError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *LiveSelectionError) Error() string { return "live selection: " + e.Code }
func (e *LiveSelectionError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

// Only errors from owned stores, verifiers and controls enter this boundary.
// Each of those boundaries already sanitizes its raw backend/provider errors.
func liveSelectionError(ctx context.Context, err error) *LiveSelectionError {
	r := &LiveSelectionError{Code: "selection_failed"}
	switch e := err.(type) {
	case *LiveSelectionError:
		if e != nil {
			*r = *e
		}
	case *BaselineStorageError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *BaselineError:
		if e != nil {
			r.Code = e.Code
		}
	case *DiscoveryError:
		if e != nil {
			r.Code, r.HTTPStatus = e.Code, e.HTTPStatus
		}
	case *LeaseError:
		if e != nil {
			r.Code, r.HTTPStatus = e.Code, e.HTTPStatus
		}
	case *DiagnosticRunError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *OperatorRunError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *SetupError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *SetupConfirmationError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
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
