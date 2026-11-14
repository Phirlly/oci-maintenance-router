package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func preparedStoreFixture(t *testing.T) (*PreparedHistoryStore, *discoveryObjects, Baseline, Baseline) {
	t.Helper()
	first := buildBaseline(t, "initial", baselineResult(t, nil, nil, nil, nil))
	next, err := first.Extend("expanded", baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
	if err != nil {
		t.Fatal(err)
	}
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	return preparedStoreFor(t, objects, first.EnvironmentID(), "installation-1"), objects, first, next
}
func preparedStoreFor(t *testing.T, objects PreparedHistoryObjects, env, install string) *PreparedHistoryStore {
	t.Helper()
	s, err := NewPreparedHistoryStore(objects, env, install)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func advancePrepared(t *testing.T, h PreparedHistory, before, after Baseline) PreparedHistory {
	t.Helper()
	next, err := h.Advance(before, after)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func preparedStoredFailure(t *testing.T, got StoredPreparedHistory, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *BaselineStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain {
		t.Fatalf("error=%+v; want %s/%d/%t", err, code, status, uncertain)
	}
	if got.History().EnvironmentID() != "" || got.History().BaselineID() != "" {
		t.Fatal("partial handle escaped")
	}
}

type preparedBackend struct {
	get     func(context.Context, string) (ocistorage.Object, error)
	create  func(context.Context, string, []byte) (string, error)
	replace func(context.Context, string, string, []byte) (string, error)
}

func (s *preparedBackend) Get(c context.Context, n string) (ocistorage.Object, error) {
	return s.get(c, n)
}
func (s *preparedBackend) Create(c context.Context, n string, b []byte) (string, error) {
	return s.create(c, n, b)
}
func (s *preparedBackend) Replace(c context.Context, n, tag string, b []byte) (string, error) {
	return s.replace(c, n, tag, b)
}
