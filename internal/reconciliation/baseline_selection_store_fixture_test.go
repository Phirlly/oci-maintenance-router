package reconciliation

import (
	"context"
	"errors"
	"testing"
)

const selectionObjectName = "control/selected-baseline.json"

type selectionBackend struct {
	*preparedBackend
	history func(context.Context, string) (bool, error)
}

func (b *selectionBackend) HistoryIsEmpty(c context.Context, n string) (bool, error) {
	return b.history(c, n)
}
func selectionStoreFor(t *testing.T, backend BaselineSelectionObjects, b Baseline) *BaselineSelectionStore {
	t.Helper()
	s, err := NewBaselineSelectionStore(backend, b.EnvironmentID(), "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func selectionStoreFixture(t *testing.T) (*BaselineSelectionStore, *discoveryObjects, *selectionBackend, Baseline, Baseline) {
	t.Helper()
	_, objects, first, second := preparedStoreFixture(t)
	backend := &selectionBackend{preparedBackend: &preparedBackend{get: objects.Get, create: objects.Create, replace: objects.Replace}, history: func(c context.Context, n string) (bool, error) {
		return true, objects.begin(c, discoveryCall{method: "HISTORY", name: n})
	}}
	return selectionStoreFor(t, backend, first), objects, backend, first, second
}
func selectionStoredFailure(t *testing.T, got StoredBaselineSelection, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *BaselineStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain {
		t.Fatalf("error=%+v; want %s/%d/%t", err, code, status, uncertain)
	}
	if got.Selection().EnvironmentID() != "" || got.Selection().BaselineID() != "" {
		t.Fatal("failure returned partial handle")
	}
}
func selectionSaved(t *testing.T, s *BaselineSelectionStore, b Baseline) StoredBaselineSelection {
	t.Helper()
	got, err := s.Create(leaseBudget(t), selectionFor(t, b))
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func advanceSelection(t *testing.T, s BaselineSelection, previous, next Baseline) BaselineSelection {
	t.Helper()
	got, err := s.Advance(previous, next)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
