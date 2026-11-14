package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// A run checks its original work reserve at every actual dispatch, including
// retained-history probes and conditional-write confirmations. No detached work.
type liveSelectionStorage struct {
	objects BaselineSelectionObjects
	reserve func() error
}

func (s liveSelectionStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s liveSelectionStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Create(ctx, name, body)
}
func (s liveSelectionStorage) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Replace(ctx, name, tag, body)
}
func (s liveSelectionStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}
