package reconciliation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

type PreparationIssueObjects interface {
	Get(context.Context, string) (ocistorage.Object, error)
	Create(context.Context, string, []byte) (string, error)
	Replace(context.Context, string, string, []byte) (string, error)
	HistoryIsEmpty(context.Context, string) (bool, error)
}
type PreparationIssueStore struct {
	objects     PreparationIssueObjects
	environment string
}
type StoredPreparationIssues struct {
	store  *PreparationIssueStore
	notice noticeledger.Record
	record noticeledger.PreparationIssues
	etag   string
	absent bool
}

func (s StoredPreparationIssues) Record() noticeledger.PreparationIssues { return s.record }
func NewPreparationIssueStore(objects PreparationIssueObjects, environment string) (*PreparationIssueStore, error) {
	if !baselineObjectsUsable(objects, environment) {
		return nil, &PreparationIssueStorageError{Code: "invalid_request"}
	}
	return &PreparationIssueStore{objects, environment}, nil
}
func (s *PreparationIssueStore) usable() bool {
	return s != nil && baselineObjectsUsable(s.objects, s.environment)
}
func preparationIssueName(r noticeledger.Record) string {
	_, id := r.Identity()
	return fmt.Sprintf("review/preparation/%x.json", sha256.Sum256([]byte(id)))
}

var _ PreparationIssueObjects = (*ocistorage.Client)(nil)
