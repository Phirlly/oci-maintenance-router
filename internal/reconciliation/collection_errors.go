package reconciliation

import (
	"context"
	"errors"
)

// CollectionError retains only controlled diagnostics and standard cancellation
// identity. Provider errors, object names and source text are never retained.
type CollectionError Error

func (e *CollectionError) Error() string { return "collection: " + string(e.Code) }
func (e *CollectionError) Unwrap() error { return (*Error)(e).Unwrap() }

func collectionFailure(ctx context.Context, source Source, err error) *CollectionError {
	code, status := InvalidRequest, 0
	var enumerationErr *Error
	var candidateErr *CandidateError
	if errors.As(err, &enumerationErr) && enumerationErr != nil {
		code, source, status = enumerationErr.Code, enumerationErr.Source, enumerationErr.HTTPStatus
	} else if errors.As(err, &candidateErr) && candidateErr != nil {
		code = candidateErr.Code
	}
	failure := failure(ctx, source, code)
	failure.HTTPStatus = status
	return (*CollectionError)(failure)
}
