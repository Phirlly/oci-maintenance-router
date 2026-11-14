package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
)

// Load never initializes or interprets a missing current object as a fresh request.
func (s *DiagnosticStore) Load(ctx context.Context, request string) (StoredDiagnostic, error) {
	if !s.usable() || !diagnosticID.MatchString(request) {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := diagnosticStoreContext(ctx)
	if err != nil {
		return StoredDiagnostic{}, err
	}
	defer cancel()
	got, _, failure := s.load(ctx, request)
	if failure != nil {
		return StoredDiagnostic{}, failure
	}
	return got, nil
}
func (s *DiagnosticStore) load(ctx context.Context, request string) (StoredDiagnostic, bool, *DiagnosticStorageError) {
	if ctx.Err() != nil {
		return StoredDiagnostic{}, false, diagnosticStorageFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, s.name(request))
	if err != nil {
		service, ok := err.(*ocistorage.Error)
		missing := ctx.Err() == nil && ok && service != nil && service.Code == ocistorage.Service && service.Status == 404
		return StoredDiagnostic{}, missing, diagnosticStorageFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return StoredDiagnostic{}, false, diagnosticStorageFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return StoredDiagnostic{}, false, &DiagnosticStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	r, err := DecodeDiagnostic(obj.Body, s.environment, s.installation, request)
	if err != nil || ctx.Err() != nil {
		return StoredDiagnostic{}, false, diagnosticStorageFailure(ctx, "invalid_record", 200, false)
	}
	return StoredDiagnostic{s, r, strings.Clone(obj.ETag)}, false, nil
}
