package reconciliation

import "testing"

func TestOperatorChallengeSaveBindsExactParentAndStore(t *testing.T) {
	s, objects, b := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	ctx := leaseBudget(t)
	separatelyLoaded, err := s.Load(ctx, "operator-test-1")
	if err != nil {
		t.Fatal(err)
	}
	next := operatorOutcome(t, old, 200)
	body, _ := next.Encode()
	i := operatorIntentFixture(t)
	decoded, err := DecodeOperatorChallenge(body, i.EnvironmentID, i.InstallationID, i.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(objects.calls)
	for _, tc := range []struct {
		store *OperatorChallengeStore
		old   StoredOperatorChallenge
		next  OperatorChallenge
	}{
		{s, separatelyLoaded, next}, {s, old, decoded}, {s, old, old.Record()}, {s, StoredOperatorChallenge{}, next}, {s, old, OperatorChallenge{}}, {operatorStoreFor(t, b), old, next}, {nil, old, next}, {&OperatorChallengeStore{}, old, next},
	} {
		got, err := tc.store.Save(ctx, tc.old, tc.next)
		operatorStoredFailure(t, got, err, "invalid_request", 0, false)
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
	different := operatorOutcome(t, old, 400)
	got, err := s.Save(ctx, old, different)
	operatorStoredFailure(t, got, err, "identity_conflict", 200, false)
	if objects.version != 2 {
		t.Fatal("competing terminal result overwrote winner")
	}
}

func TestOperatorChallengeMissingSaveNeverCreates(t *testing.T) {
	s, objects, _ := operatorStoreFixture(t)
	old := operatorSaved(t, s)
	objects.mu.Lock()
	delete(objects.objects, operatorObjectName)
	objects.mu.Unlock()
	got, err := s.Save(leaseBudget(t), old, operatorOutcome(t, old, 200))
	operatorStoredFailure(t, got, err, "read_failed", 404, false)
	if objects.version != 1 {
		t.Fatal("missing claim recreated during save")
	}
}
