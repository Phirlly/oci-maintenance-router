package reconciliation

import (
	"context"
	"errors"

	"oci-maintenance-router/internal/ocistorage"
)

// DiscoveryError contains controlled diagnostics, never backend/source content.
type DiscoveryError struct {
	Code       string
	HTTPStatus int
}

func (e *DiscoveryError) Error() string { return "discovery: " + e.Code }
func (e *DiscoveryError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

func discoveryFailureFor(ctx context.Context, code string, err error) *DiscoveryError {
	result := &DiscoveryError{Code: code}
	var own *DiscoveryError
	var lease *LeaseError
	var collection *CollectionError
	var storage *ocistorage.Error
	var enumeration *Error
	switch {
	case errors.As(err, &own) && own != nil:
		*result = *own
	case errors.As(err, &lease) && lease != nil:
		result.Code, result.HTTPStatus = lease.Code, lease.HTTPStatus
	case errors.As(err, &collection) && collection != nil:
		result.HTTPStatus = collection.HTTPStatus
		switch collection.Code {
		case Canceled, DeadlineExceeded, Code("insufficient_time"):
			result.Code = string(collection.Code)
		}
	case errors.As(err, &enumeration) && enumeration != nil:
		result.Code = string(enumeration.Code)
	case errors.As(err, &storage) && storage != nil:
		result.HTTPStatus = storage.Status
	}
	if result.HTTPStatus < 100 || result.HTTPStatus > 599 {
		result.HTTPStatus = 0
	}
	if ctx != nil {
		switch ctx.Err() {
		case context.Canceled:
			result.Code = "canceled"
		case context.DeadlineExceeded:
			result.Code = "deadline_exceeded"
		}
	}
	return result
}
