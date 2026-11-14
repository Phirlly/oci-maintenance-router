package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

type baselineBackend struct {
	get    func(context.Context, string) (ocistorage.Object, error)
	create func(context.Context, string, []byte) (string, error)
}

func (s *baselineBackend) Get(c context.Context, n string) (ocistorage.Object, error) {
	return s.get(c, n)
}
func (s *baselineBackend) Create(c context.Context, n string, b []byte) (string, error) {
	return s.create(c, n, b)
}
func baselineStoreFixture(t *testing.T) (*BaselineStore, *discoveryObjects, Baseline) {
	t.Helper()
	b := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	s := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	return baselineStoreFor(t, s, b.EnvironmentID()), s, b
}
func baselineStoreFor(t *testing.T, s BaselineObjects, env string) *BaselineStore {
	t.Helper()
	store, err := NewBaselineStore(s, env)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func baselineStoredFailure(t *testing.T, got Baseline, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *BaselineStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain {
		t.Fatalf("error=%+v; want %s/%d/%t", err, code, status, uncertain)
	}
	if got.ID() != "" || got.EnvironmentID() != "" || got.Layers() != nil {
		t.Fatal("failure returned partial baseline")
	}
}
