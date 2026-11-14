package reconciliation

import (
	"context"
	"reflect"

	"oci-maintenance-router/internal/ocistorage"
)

// BaselineObjects provides owned bounded responses and conditional creation,
// honoring caller deadlines without retries. Bootstrap binds the location to
// the store's environment. No replacement or deletion is available here.
type BaselineObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
}

// BaselineStore confirms immutable storage, never discovery, operator intent,
// selection, lease ownership, activation or permission to send.
type BaselineStore struct {
	objects       BaselineObjects
	environmentID string
}

func NewBaselineStore(objects BaselineObjects, environmentID string) (*BaselineStore, error) {
	s := &BaselineStore{objects: objects, environmentID: environmentID}
	if !s.usable() {
		return nil, &BaselineStorageError{Code: "invalid_request"}
	}
	return s, nil
}
func (s *BaselineStore) usable() bool {
	if s == nil || s.objects == nil || !leaseDigest.MatchString(s.environmentID) {
		return false
	}
	v := reflect.ValueOf(s.objects)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

func (s *BaselineStore) Load(ctx context.Context, id string) (Baseline, error) {
	if !s.usable() || !baselineID.MatchString(id) {
		return Baseline{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return Baseline{}, err
	}
	defer cancel()
	b, failure := s.load(ctx, id)
	if failure != nil {
		return Baseline{}, failure
	}
	return b, nil
}

func (s *BaselineStore) load(ctx context.Context, id string) (Baseline, *BaselineStorageError) {
	if ctx.Err() != nil {
		return Baseline{}, baselineStorageFailure(ctx, "read_failed", 0, false)
	}
	obj, err := s.objects.Get(ctx, "baselines/"+id+".json")
	if err != nil {
		return Baseline{}, baselineStorageFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if ctx.Err() != nil {
		return Baseline{}, baselineStorageFailure(ctx, "read_failed", 200, false)
	}
	if !baselineETag(obj.ETag) {
		return Baseline{}, &BaselineStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	b, err := DecodeBaseline(obj.Body, s.environmentID, id)
	if err != nil || ctx.Err() != nil {
		return Baseline{}, baselineStorageFailure(ctx, "invalid_record", 200, false)
	}
	return b, nil
}
