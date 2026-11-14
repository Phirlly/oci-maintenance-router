package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

type ProcessingError struct {
	Code           string
	HTTPStatus     int
	WriteUncertain bool
}

func (e *ProcessingError) Error() string { return "maintenance processing: " + e.Code }
func (e *ProcessingError) Unwrap() error {
	switch e.Code {
	case "canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	}
	return nil
}

// Raw source, ownership, credential and transport errors never enter this mapper.
// Owned coordinators/stores have already sanitized their adapter boundaries.
func processingFailure(ctx context.Context, err error) *ProcessingError {
	r := &ProcessingError{Code: "processing_failed"}
	switch e := err.(type) {
	case *ProcessingError:
		if e != nil {
			*r = *e
		}
	case *ocistorage.Error:
		if e != nil {
			r.Code = "state_failed"
			r.HTTPStatus = e.Status
			r.WriteUncertain = e.WriteUncertain
			if e.Code == ocistorage.Conflict {
				r.Code = "conflict"
			}
		}
	case *PreparationIssueStorageError:
		if e != nil {
			r.Code, r.HTTPStatus, r.WriteUncertain = "issue_"+e.Code, e.HTTPStatus, e.WriteUncertain
		}
	default:
		detail := liveSelectionError(ctx, err)
		if detail.Code != "selection_failed" {
			r.Code, r.HTTPStatus, r.WriteUncertain = detail.Code, detail.HTTPStatus, detail.WriteUncertain
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
