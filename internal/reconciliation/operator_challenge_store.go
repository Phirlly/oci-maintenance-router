package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// OperatorChallengeObjects honors caller lifetimes and conditional writes without
// retries. Trusted bootstrap binds this backend and retained history to the
// environment. A storage result is never permission to publish a challenge.
type OperatorChallengeObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Replace(context.Context, string, string, []byte) (string, error)
	HistoryIsEmpty(context.Context, string) (bool, error)
}
type OperatorChallengeStore struct {
	objects                   OperatorChallengeObjects
	environment, installation string
}

// StoredOperatorChallenge binds the exact observed record/store/ETag. In particular,
// loading an existing PUBLISHING record grants no new claim or dispatch authority.
type StoredOperatorChallenge struct {
	store  *OperatorChallengeStore
	record OperatorChallenge
	etag   string
}

func NewOperatorChallengeStore(objects OperatorChallengeObjects, environment, installation string) (*OperatorChallengeStore, error) {
	s := &OperatorChallengeStore{objects, environment, installation}
	if !s.usable() {
		return nil, &OperatorChallengeStorageError{Code: "invalid_request"}
	}
	return s, nil
}
func (s *OperatorChallengeStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment) && operatorLocalID.MatchString(s.installation)
}
func (s *OperatorChallengeStore) name(request string) string {
	return "control/operator-challenges/" + s.installation + "/" + request + ".json"
}
func (s StoredOperatorChallenge) Record() OperatorChallenge { return s.record }

var _ OperatorChallengeObjects = (*ocistorage.Client)(nil)
