package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

const operatorMessageID = "550e8400-e29b-41d4-a716-446655440000"

const operatorObjectName = "control/operator-challenges/installation-1/operator-test-1.json"

type operatorBackend struct {
	*preparedBackend
	history func(context.Context, string) (bool, error)
}

func (b *operatorBackend) HistoryIsEmpty(c context.Context, n string) (bool, error) {
	return b.history(c, n)
}
func operatorStoreFixture(t *testing.T) (*OperatorChallengeStore, *discoveryObjects, *operatorBackend) {
	t.Helper()
	objects := &discoveryObjects{objects: map[string]ocistorage.Object{}}
	backend := &operatorBackend{preparedBackend: &preparedBackend{get: objects.Get, create: objects.Create, replace: objects.Replace}, history: func(c context.Context, n string) (bool, error) {
		return true, objects.begin(c, discoveryCall{method: "HISTORY", name: n})
	}}
	return operatorStoreFor(t, backend), objects, backend
}
func operatorStoreFor(t *testing.T, backend OperatorChallengeObjects) *OperatorChallengeStore {
	t.Helper()
	i := operatorIntentFixture(t)
	s, err := NewOperatorChallengeStore(backend, i.EnvironmentID, i.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func operatorSaved(t *testing.T, s *OperatorChallengeStore) StoredOperatorChallenge {
	t.Helper()
	got, err := s.Create(leaseBudget(t), operatorChallengeFixture(t))
	if err != nil || got.Record().Snapshot().State != OperatorPublishing {
		t.Fatal("valid claim not stored", err)
	}
	return got
}
func operatorStoredFailure(t *testing.T, got StoredOperatorChallenge, err error, code string, status int, uncertain bool) {
	t.Helper()
	var e *OperatorChallengeStorageError
	if !errors.As(err, &e) || e.Code != code || e.HTTPStatus != status || e.WriteUncertain != uncertain {
		t.Fatalf("error=%+v; want %s/%d/%t", err, code, status, uncertain)
	}
	if got.Record().Snapshot().State != "" || got.Record().Snapshot().Payload != nil {
		t.Fatal("failed storage returned partial handle")
	}
}
func operatorOutcome(t *testing.T, old StoredOperatorChallenge, status int) OperatorChallenge {
	t.Helper()
	c := operatorClaimFixture()
	messageID := ""
	if status == 200 {
		messageID = operatorMessageID
	}
	next, err := old.Record().Finish(operatorReference(c), status, messageID, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
