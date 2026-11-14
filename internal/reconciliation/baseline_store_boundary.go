package reconciliation

import (
	"context"
	"strings"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

// BaselineStorageError reports controlled storage failure without object data.
type BaselineStorageError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *BaselineStorageError) Error() string { return "baseline storage: " + e.Code }
func (e *BaselineStorageError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

func baselineStoreContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, &BaselineStorageError{Code: "invalid_request"}
	}
	if ctx.Err() != nil {
		return nil, nil, baselineStorageFailure(ctx, "canceled", 0, false)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, nil, &BaselineStorageError{Code: "deadline_required"}
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	return bounded, cancel, nil
}

func baselineStorageFailure(ctx context.Context, code string, status int, uncertain bool) *BaselineStorageError {
	switch ctx.Err() {
	case context.Canceled:
		code = "canceled"
	case context.DeadlineExceeded:
		code = "deadline_exceeded"
	}
	return &BaselineStorageError{Code: code, HTTPStatus: status, WriteUncertain: uncertain}
}
func baselineStorageStatus(err error) int {
	if e, ok := err.(*ocistorage.Error); ok && e != nil && e.Status >= 100 && e.Status <= 599 {
		return e.Status
	}
	return 0
}

// Do not inspect wrapped/unknown backend errors or trust arbitrary exported Codes.
func baselineWriteDisposition(err error) (confirm, uncertain bool) {
	e, ok := err.(*ocistorage.Error)
	if !ok || e == nil {
		return false, true
	}
	if e.Code == ocistorage.Conflict && e.Status == 412 {
		return true, e.WriteUncertain
	}
	if e.Code == ocistorage.Service && e.Status >= 400 && e.Status < 500 {
		return false, false
	}
	switch e.Code {
	case ocistorage.InvalidConfiguration, ocistorage.InvalidRequest, ocistorage.DeadlineRequired,
		ocistorage.Canceled, ocistorage.DeadlineExceeded, ocistorage.Authentication,
		ocistorage.Transport, ocistorage.InvalidResponse, ocistorage.TooLarge,
		ocistorage.Conflict, ocistorage.Service:
		return e.WriteUncertain, e.WriteUncertain
	}
	return false, true
}

// Tags are validated only as storage-response evidence, never retained as a
// replacement capability. Keep the concrete adapter's conservative strong shape.
func baselineETag(tag string) bool {
	if len(tag) == 0 || len(tag) > 1024 || strings.HasPrefix(tag, "W/") {
		return false
	}
	if len(tag) >= 2 && tag[0] == '"' && tag[len(tag)-1] == '"' {
		tag = tag[1 : len(tag)-1]
	}
	if tag == "" {
		return false
	}
	for _, c := range tag {
		if c <= 32 || c >= 127 || strings.ContainsRune("\"\\,*", c) {
			return false
		}
	}
	return true
}
