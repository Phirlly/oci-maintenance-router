package reconciliation

import "testing"

func TestDiagnosticSaveBindsExactParentAndStore(t *testing.T) {
	s, objects, b := diagnosticStoreFixture(t)
	old := diagnosticSaved(t, s)
	ctx := leaseBudget(t)
	separatelyLoaded, err := s.Load(ctx, "test-1")
	if err != nil {
		t.Fatal(err)
	}
	next := diagnosticOutcome(t, old, 202)
	body, _ := next.Encode()
	i := diagnosticIntentFixture()
	decoded, err := DecodeDiagnostic(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(objects.calls)
	for _, tc := range []struct {
		store *DiagnosticStore
		old   StoredDiagnostic
		next  DiagnosticRecord
	}{
		{s, separatelyLoaded, next}, {s, old, decoded}, {s, old, old.Record()}, {s, StoredDiagnostic{}, next}, {s, old, DiagnosticRecord{}}, {diagnosticStoreFor(t, b), old, next}, {nil, old, next}, {&DiagnosticStore{}, old, next},
	} {
		got, err := tc.store.Save(ctx, tc.old, tc.next)
		diagnosticStoredFailure(t, got, err, "invalid_request", 0, false)
	}
	if len(objects.calls) != before {
		t.Fatal("unbound save dispatched")
	}
	saved, err := s.Save(ctx, old, next)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.Save(ctx, old, next)
	if err != nil || repeated.Record().Snapshot().State != saved.Record().Snapshot().State || objects.version != 2 {
		t.Fatal("exact repeated save not confirmed", err)
	}
	different := diagnosticOutcome(t, old, 400)
	got, err := s.Save(ctx, old, different)
	diagnosticStoredFailure(t, got, err, "identity_conflict", 200, false)
	if objects.version != 2 {
		t.Fatal("competing terminal result overwrote winner")
	}
}

func TestDiagnosticMissingSaveNeverCreates(t *testing.T) {
	s, objects, _ := diagnosticStoreFixture(t)
	old := diagnosticSaved(t, s)
	objects.mu.Lock()
	delete(objects.objects, diagnosticObjectName)
	objects.mu.Unlock()
	got, err := s.Save(leaseBudget(t), old, diagnosticOutcome(t, old, 202))
	diagnosticStoredFailure(t, got, err, "read_failed", 404, false)
	if objects.version != 1 {
		t.Fatal("missing claim recreated during save")
	}
}
