package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// Each run owns this wrapper and uses it synchronously. Outcome persistence
// replaces its work reserve with the separate receipt-only deadline check;
// the store/handle identity stays unchanged. Shared coordinator state is immutable.
type diagnosticRunStorage struct {
	objects DiagnosticObjects
	reserve func() error
}

func (s *diagnosticRunStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s *diagnosticRunStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Create(ctx, name, body)
}
func (s *diagnosticRunStorage) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Replace(ctx, name, tag, body)
}
func (s *diagnosticRunStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}
