package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionCreateRequiresEmptyRetainedHistory(t *testing.T) {
	for _, tc := range []struct {
		empty  bool
		err    error
		code   string
		status int
	}{
		{false, nil, "history_retained", 0},
		{true, errors.New("private history"), "history_read", 0},
		{false, &ocistorage.Error{Code: ocistorage.Service, Status: 403}, "history_read", 403},
	} {
		s, objects, backend, first, _ := selectionStoreFixture(t)
		backend.history = func(_ context.Context, n string) (bool, error) {
			if n != selectionObjectName {
				t.Fatal("wrong history key")
			}
			return tc.empty, tc.err
		}
		got, err := s.Create(leaseBudget(t), selectionFor(t, first))
		selectionStoredFailure(t, got, err, tc.code, tc.status, false)
		if objects.version != 0 || len(objects.calls) != 1 {
			t.Fatal("unusable history allowed write")
		}
	}
	s, objects, backend, first, _ := selectionStoreFixture(t)
	selectionSaved(t, s, first)
	delete(objects.objects, selectionObjectName)
	backend.history = func(context.Context, string) (bool, error) { return false, nil }
	got, err := s.Create(leaseBudget(t), selectionFor(t, first))
	selectionStoredFailure(t, got, err, "history_retained", 0, false)
	if objects.version != 1 {
		t.Fatal("deleted selection was silently reset")
	}
}

func TestBaselineSelectionOnlyDirectService404MayInspectHistory(t *testing.T) {
	for _, failure := range []error{errors.New("private"), &ocistorage.Error{Code: ocistorage.Service, Status: 403}, &ocistorage.Error{Code: ocistorage.InvalidResponse, Status: 404}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &BaselineStorageError{Code: "read_failed", HTTPStatus: 404}} {
		s, objects, backend, first, _ := selectionStoreFixture(t)
		reads := 0
		backend.get = func(context.Context, string) (ocistorage.Object, error) { reads++; return ocistorage.Object{}, failure }
		backend.history = func(context.Context, string) (bool, error) {
			t.Fatal("untrusted error inferred absence")
			return true, nil
		}
		got, err := s.Create(leaseBudget(t), selectionFor(t, first))
		status := 0
		if e, ok := failure.(*ocistorage.Error); ok {
			status = e.Status
		}
		selectionStoredFailure(t, got, err, "read_failed", status, false)
		if reads != 1 || objects.version != 0 {
			t.Fatal("read failure retried or changed state")
		}
	}
}
