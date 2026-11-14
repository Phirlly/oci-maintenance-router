package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// A run owns this synchronous wrapper. Only after its single publication may it
// switch to the outcome-only reserve, preserving the store and saved-handle identity.
type operatorRunStorage struct {
	objects OperatorChallengeObjects
	reserve func() error
}

func (s *operatorRunStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s *operatorRunStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Create(ctx, name, body)
}
func (s *operatorRunStorage) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Replace(ctx, name, tag, body)
}
func (s *operatorRunStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}
