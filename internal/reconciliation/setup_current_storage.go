package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

type setupCurrentStorage struct {
	objects        setupCurrentObjects
	reserve, guard func() error
}

func (s setupCurrentStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s setupCurrentStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}
func (s setupCurrentStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.guard(); err != nil {
		return "", &setupWriteBlocked{err}
	}
	return s.objects.Create(ctx, name, body)
}
func (s setupCurrentStorage) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if err := s.guard(); err != nil {
		return "", &setupWriteBlocked{err}
	}
	return s.objects.Replace(ctx, name, tag, body)
}

// Only an owned pre-dispatch check can establish that no write was attempted.
type setupWriteBlocked struct{ failure error }

func (e *setupWriteBlocked) Error() string { return "setup write blocked" }
