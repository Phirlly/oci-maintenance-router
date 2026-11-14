package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocinotifications"
)

// OperatorRunError contains controlled diagnostics, never contact or provider text.
type OperatorRunError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *OperatorRunError) Error() string { return "operator run: " + e.Code }

func (e *OperatorRunError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

// Only owned model/store/control failures enter this classifier. Channel errors
// are sanitized separately; provider text cannot forge a coordinator diagnostic.
func operatorRunError(ctx context.Context, code string, err error) *OperatorRunError {
	r := &OperatorRunError{Code: code}
	switch e := err.(type) {
	case *OperatorChallengeStorageError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = e.Code, e.HTTPStatus, e.WriteUncertain
		}
	case *OperatorChallengeError:
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

func operatorChannelReadError(ctx context.Context, err error) *OperatorRunError {
	failure := operatorRunError(ctx, "channel_read", nil)
	if e, ok := err.(*ocinotifications.Error); ok && e != nil && e.Status >= 100 && e.Status <= 599 {
		failure.HTTPStatus = e.Status
	}
	return failure
}
