package reconciliation

import "context"

// Create protects first selection with a current read and complete empty retained
// history. Exact existing bytes are a no-write repeat; other state is never reset.
// The coordinator must separately establish all LIVE/readiness/authority gates.
func (s *BaselineSelectionStore) Create(ctx context.Context, initial BaselineSelection) (StoredBaselineSelection, error) {
	if !s.usable() || initial.data == nil || initial.parent != nil || initial.EnvironmentID() != s.environment || initial.InstallationID() != s.installation {
		return StoredBaselineSelection{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return StoredBaselineSelection{}, err
	}
	defer cancel()
	got, missing, failure := s.load(ctx)
	if failure == nil {
		return selectionConfirmed(ctx, got, initial, false)
	}
	if !missing {
		return StoredBaselineSelection{}, failure
	}
	if ctx.Err() != nil {
		return StoredBaselineSelection{}, baselineStorageFailure(ctx, "history_read", 0, false)
	}
	empty, err := s.objects.HistoryIsEmpty(ctx, baselineSelectionName)
	if err != nil || ctx.Err() != nil {
		return StoredBaselineSelection{}, baselineStorageFailure(ctx, "history_read", baselineStorageStatus(err), false)
	}
	if !empty {
		return StoredBaselineSelection{}, &BaselineStorageError{Code: "history_retained"}
	}
	return s.write(ctx, "", initial)
}
