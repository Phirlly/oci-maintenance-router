package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func TestDiagnosticCreateRequiresCompleteEmptyHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		empty  bool
		err    error
		code   string
		status int
	}{
		{"retained version or delete marker", false, nil, "history_retained", 0},
		{"failed empty response", true, errors.New("private"), "history_read", 0},
		{"permission denied", false, &ocistorage.Error{Code: ocistorage.Service, Status: 403}, "history_read", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, objects, b := diagnosticStoreFixture(t)
			b.history = func(context.Context, string) (bool, error) { return tc.empty, tc.err }
			got, err := s.Create(leaseBudget(t), diagnosticFixture(t))
			diagnosticStoredFailure(t, got, err, tc.code, tc.status, false)
			if objects.version != 0 || len(objects.calls) != 1 {
				t.Fatal("unusable history allowed write")
			}
		})
	}
}

func TestDiagnosticCreateOnlyDirectService404MayInspectHistory(t *testing.T) {
	for _, failure := range []error{errors.New("private"), &ocistorage.Error{Code: ocistorage.Service, Status: 403}, &ocistorage.Error{Code: ocistorage.InvalidResponse, Status: 404}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &DiagnosticStorageError{Code: "read_failed", HTTPStatus: 404}} {
		s, objects, b := diagnosticStoreFixture(t)
		reads := 0
		b.get = func(context.Context, string) (ocistorage.Object, error) { reads++; return ocistorage.Object{}, failure }
		b.history = func(context.Context, string) (bool, error) {
			t.Fatal("untrusted failure inferred missing request")
			return true, nil
		}
		got, err := s.Create(leaseBudget(t), diagnosticFixture(t))
		status := 0
		if e, ok := failure.(*ocistorage.Error); ok {
			status = e.Status
		}
		diagnosticStoredFailure(t, got, err, "read_failed", status, false)
		if reads != 1 || objects.version != 0 {
			t.Fatal("failed read mutated state")
		}
	}
}

func TestDiagnosticDeletedConsumedRequestIsNotRecreated(t *testing.T) {
	s, objects, b := diagnosticStoreFixture(t)
	diagnosticSaved(t, s)
	objects.mu.Lock()
	delete(objects.objects, diagnosticObjectName)
	objects.mu.Unlock()
	b.history = func(_ context.Context, name string) (bool, error) {
		if name != diagnosticObjectName {
			t.Fatal("wrong history prefix")
		}
		return false, nil
	}
	got, err := s.Create(leaseBudget(t), diagnosticFixture(t))
	diagnosticStoredFailure(t, got, err, "history_retained", 0, false)
	if objects.version != 1 {
		t.Fatal("consumed request silently recreated")
	}
}
