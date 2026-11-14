package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/ocistorage"
)

const baselineSelectionName = "control/selected-baseline.json"

// BaselineSelectionObjects honors bounded caller lifetimes and conditional writes
// without retries. Bootstrap binds this backend and retained history to location.
type BaselineSelectionObjects interface {
	PreparedHistoryObjects
	HistoryIsEmpty(context.Context, string) (bool, error)
}

// BaselineSelectionStore confirms selection bytes only. It does not establish
// LIVE intent, baseline persistence/coverage, readiness, CURRENT controls or lease.
type BaselineSelectionStore struct {
	objects                   BaselineSelectionObjects
	environment, installation string
}

// StoredBaselineSelection binds an exact observed record/store/ETag. It grants
// no activation or send authority, even when an earlier selection is unchanged.
type StoredBaselineSelection struct {
	store     *BaselineSelectionStore
	selection BaselineSelection
	etag      string
}

func NewBaselineSelectionStore(objects BaselineSelectionObjects, environment, installation string) (*BaselineSelectionStore, error) {
	s := &BaselineSelectionStore{objects, environment, installation}
	if !s.usable() {
		return nil, &BaselineStorageError{Code: "invalid_request"}
	}
	return s, nil
}
func (s *BaselineSelectionStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && baselineID.MatchString(s.installation)
}
func (s StoredBaselineSelection) Selection() BaselineSelection { return s.selection }

var _ BaselineSelectionObjects = (*ocistorage.Client)(nil)
