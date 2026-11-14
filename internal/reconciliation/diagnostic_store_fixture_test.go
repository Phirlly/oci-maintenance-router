package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

const diagnosticObjectName = "control/diagnostics/installation-1/test-1.json"

type diagnosticBackend struct {
	*preparedBackend
	history func(context.Context, string) (bool, error)
}

func (b *diagnosticBackend) HistoryIsEmpty(c context.Context, n string) (bool, error) {
	return b.history(c, n)
}
func diagnosticStoreFixture(t *testing.T) (*DiagnosticStore, *discoveryObjects, *diagnosticBackend) {
	t.Helper()
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	backend := &diagnosticBackend{preparedBackend: &preparedBackend{get: objects.Get, create: objects.Create, replace: objects.Replace}, history: func(c context.Context, n string) (bool, error) {
		return true, objects.begin(c, discoveryCall{method: "HISTORY", name: n})
	}}
	return diagnosticStoreFor(t, backend), objects, backend
}
func diagnosticStoreFor(t *testing.T, backend DiagnosticObjects) *DiagnosticStore {
	t.Helper()
	i := diagnosticIntentFixture()
	s, err := NewDiagnosticStore(backend, i.EnvironmentID, i.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func diagnosticSaved(t *testing.T, s *DiagnosticStore) StoredDiagnostic {
	t.Helper()
	got, err := s.Create(leaseBudget(t), diagnosticFixture(t))
	if err != nil || got.Record().Snapshot().State != DiagnosticSending {
		t.Fatal("valid claim not stored", err)
	}
	return got
}
func diagnosticStoredFailure(t *testing.T, got StoredDiagnostic, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *DiagnosticStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain {
		t.Fatalf("error=%+v; want %s/%d/%t", err, code, status, uncertain)
	}
	if got.Record().Snapshot().State != "" || got.Record().Snapshot().PayloadWithoutRoutingKey != nil {
		t.Fatal("failed storage returned partial handle")
	}
}
func diagnosticOutcome(t *testing.T, old StoredDiagnostic, status int) DiagnosticRecord {
	t.Helper()
	c := diagnosticClaimFixture()
	next, err := old.Record().Finish(diagnosticReference(c), status, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
