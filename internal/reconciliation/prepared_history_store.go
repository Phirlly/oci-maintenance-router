package reconciliation

import (
	"context"
	"strings"

	"oci-maintenance-router/internal/ocistorage"
)

const preparedHistoryName = "control/prepared-history.json"

// PreparedHistoryObjects honors caller lifetimes and conditional writes without
// retries. Trusted bootstrap binds this backend to the environment's location.
type PreparedHistoryObjects interface {
	BaselineObjects
	Replace(context.Context, string, string, []byte) (string, error)
}
type PreparedHistoryStore struct {
	objects                   PreparedHistoryObjects
	environment, installation string
}

// StoredPreparedHistory binds one observed version, not initialization,
// preparation, activation or send authority.
type StoredPreparedHistory struct {
	store   *PreparedHistoryStore
	history PreparedHistory
	etag    string
}

func NewPreparedHistoryStore(objects PreparedHistoryObjects, environment, installation string) (*PreparedHistoryStore, error) {
	s := &PreparedHistoryStore{objects, environment, installation}
	if !s.usable() {
		return nil, &BaselineStorageError{Code: "invalid_request"}
	}
	return s, nil
}
func (s *PreparedHistoryStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && baselineID.MatchString(s.installation)
}
func (s StoredPreparedHistory) History() PreparedHistory { return s.history }

// Load never initializes missing history.
func (s *PreparedHistoryStore) Load(ctx context.Context) (StoredPreparedHistory, error) {
	if !s.usable() {
		return StoredPreparedHistory{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return StoredPreparedHistory{}, err
	}
	defer cancel()
	got, failure := s.load(ctx)
	if failure != nil {
		return StoredPreparedHistory{}, failure
	}
	return got, nil
}
func (s *PreparedHistoryStore) load(ctx context.Context) (StoredPreparedHistory, *BaselineStorageError) {
	if ctx.Err() != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, preparedHistoryName)
	if err != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return StoredPreparedHistory{}, &BaselineStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	h, err := DecodePreparedHistory(obj.Body, s.environment, s.installation)
	if err != nil || ctx.Err() != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "invalid_record", 200, false)
	}
	return StoredPreparedHistory{s, h, strings.Clone(obj.ETag)}, nil
}

var _ PreparedHistoryObjects = (*ocistorage.Client)(nil)
