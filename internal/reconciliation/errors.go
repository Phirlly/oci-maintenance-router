package reconciliation

import (
	"context"
	"errors"

	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
)

type Code string
type Source string

const (
	Announcements Source = "announcements"
	SavedNotices  Source = "saved_notices"

	InvalidRequest   Code = "invalid_request"
	DeadlineRequired Code = "deadline_required"
	Canceled         Code = "canceled"
	DeadlineExceeded Code = "deadline_exceeded"
	ReadFailed       Code = "read_failed"
	InvalidPage      Code = "invalid_page"
	CursorCycle      Code = "cursor_cycle"
	PageLimit        Code = "page_limit"
	ItemLimit        Code = "item_limit"
	ByteLimit        Code = "byte_limit"
)

// Error exposes only controlled stage/code and a valid HTTP status, never a
// cursor, identifier, provider error or response. No error proves empty scope.
type Error struct {
	Code       Code
	Source     Source
	HTTPStatus int
}

func (e *Error) Error() string { return "enumeration: " + string(e.Code) }

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

func failure(ctx context.Context, source Source, code Code) *Error {
	switch ctx.Err() {
	case context.Canceled:
		code = Canceled
	case context.DeadlineExceeded:
		code = DeadlineExceeded
	}
	return &Error{Code: code, Source: source}
}

func readFailure(ctx context.Context, source Source, err error) *Error {
	result := failure(ctx, source, ReadFailed)
	var a *ociannouncements.Error
	var s *ocistorage.Error
	if source == Announcements && errors.As(err, &a) && a != nil {
		result.HTTPStatus = a.Status
	} else if source == SavedNotices && errors.As(err, &s) && s != nil {
		result.HTTPStatus = s.Status
	}
	if result.HTTPStatus < 100 || result.HTTPStatus > 599 {
		result.HTTPStatus = 0
	}
	return result
}
