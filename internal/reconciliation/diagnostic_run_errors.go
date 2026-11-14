package reconciliation

import "context"

// DiagnosticRunError excludes provider errors and all credential contents.
type DiagnosticRunError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *DiagnosticRunError) Error() string { return "diagnostic run: " + e.Code }
func (e *DiagnosticRunError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

// Only owned model/store/control failures enter this classifier. Raw credential
// errors use a fixed category at their call site; backend text is never copied.
func diagnosticRunError(ctx context.Context, code string, err error) *DiagnosticRunError {
	r := &DiagnosticRunError{Code: code}
	switch e := err.(type) {
	case *DiagnosticStorageError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *DiagnosticError:
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
