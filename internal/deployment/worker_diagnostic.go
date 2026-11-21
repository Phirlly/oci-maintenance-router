package deployment

import (
	"errors"

	"oci-maintenance-router/internal/reconciliation"
)

// Worker diagnostics are log-only. They never grant ownership or change the
// public result, and carry no provider text, object identity or lease token.
type workerDiagnostic struct {
	Stage           string                 `json:"stage"`
	Code            string                 `json:"code"`
	Request         string                 `json:"request,omitempty"`
	StorageCategory string                 `json:"storageCategory,omitempty"`
	HTTPStatus      int                    `json:"httpStatus,omitempty"`
	WriteUncertain  bool                   `json:"writeUncertain,omitempty"`
	PriorWrite      *workerWriteDiagnostic `json:"priorWrite,omitempty"`
}

type workerWriteDiagnostic struct {
	Category   string `json:"category"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}

type ownershipGuardError struct{ cause error }

func (*ownershipGuardError) Error() string { return "ownership_failed" }

func describeOwnershipGuard(stage string, err error) *workerDiagnostic {
	guard, ok := err.(*ownershipGuardError)
	if !ok || guard == nil {
		return nil
	}
	return describeWorkerFailure(stage, guard.cause)
}

func describeWorkerFailure(stage string, err error) *workerDiagnostic {
	diagnostic := &workerDiagnostic{Stage: "unknown", Code: "unknown"}
	switch stage {
	case "acquire", "current_before", "current_after", "release":
		diagnostic.Stage = stage
	}
	var detail *reconciliation.LeaseError
	if !errors.As(err, &detail) || detail == nil {
		return diagnostic
	}
	switch detail.Code {
	case "invalid_request", "clock_invalid", "read_failed", "write_failed",
		"invalid_response", "ownership_lost", "conflict", "invalid_record",
		"canceled", "deadline_exceeded", "deadline_required", "insufficient_time":
		diagnostic.Code = detail.Code
	default:
		return diagnostic
	}
	switch detail.Request {
	case "read", "create", "replace", "create_confirmation", "replace_confirmation":
		diagnostic.Request = detail.Request
	}
	diagnostic.WriteUncertain = detail.WriteUncertain
	if detail.StorageCategory != "" {
		storage := describeWorkerStorage(detail.StorageCategory, detail.HTTPStatus)
		diagnostic.StorageCategory, diagnostic.HTTPStatus = storage.Category, storage.HTTPStatus
	}
	if detail.PriorWrite != nil {
		prior := detail.PriorWrite
		diagnostic.PriorWrite = describeWorkerStorage(prior.Category, prior.HTTPStatus)
	}
	return diagnostic
}

func describeWorkerStorage(category string, status int) *workerWriteDiagnostic {
	diagnostic := &workerWriteDiagnostic{Category: "unknown"}
	switch category {
	case "invalid_configuration", "invalid_request", "deadline_required", "canceled",
		"deadline_exceeded", "authentication", "transport", "invalid_response",
		"too_large", "conflict", "service":
		diagnostic.Category = category
		if status >= 100 && status <= 599 {
			diagnostic.HTTPStatus = status
		}
	}
	return diagnostic
}
