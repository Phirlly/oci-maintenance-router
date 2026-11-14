package reconciliation

import (
	"context"
	"strings"

	"oci-maintenance-router/internal/ocistorage"
)

// Load never initializes or repairs missing, corrupt or foreign state.
func (s *BaselineSelectionStore) Load(ctx context.Context) (StoredBaselineSelection, error) {
	if !s.usable() {
		return StoredBaselineSelection{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return StoredBaselineSelection{}, err
	}
	defer cancel()
	got, _, failure := s.load(ctx)
	if failure != nil {
		return StoredBaselineSelection{}, failure
	}
	return got, nil
}
func (s *BaselineSelectionStore) load(ctx context.Context) (StoredBaselineSelection, bool, *BaselineStorageError) {
	if ctx.Err() != nil {
		return StoredBaselineSelection{}, false, baselineStorageFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, baselineSelectionName)
	if err != nil {
		service, ok := err.(*ocistorage.Error)
		missing := ctx.Err() == nil && ok && service != nil && service.Code == ocistorage.Service && service.Status == 404
		return StoredBaselineSelection{}, missing, baselineStorageFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return StoredBaselineSelection{}, false, baselineStorageFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return StoredBaselineSelection{}, false, &BaselineStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	selection, err := DecodeBaselineSelection(obj.Body, s.environment, s.installation)
	if err != nil || ctx.Err() != nil {
		return StoredBaselineSelection{}, false, baselineStorageFailure(ctx, "invalid_record", 200, false)
	}
	return StoredBaselineSelection{s, selection, strings.Clone(obj.ETag)}, false, nil
}
