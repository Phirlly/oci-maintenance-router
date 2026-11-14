package reconciliation

import (
	"context"

	"oci-maintenance-router/internal/ocistorage"
)

// Check the run's original budget at the actual dispatch, including after
// NoticeStore encoding/validation. Store operation contexts retain their shorter
// deadline; requiring a fresh ten seconds from those child contexts would reject
// every call. No remote control lookup or automatic retry occurs here.
type discoveryStorage struct {
	objects ocistorage.NoticeObjects
	reserve func() error
}

func (s discoveryStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s discoveryStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Create(ctx, name, body)
}
func (s discoveryStorage) Replace(ctx context.Context, name, tag string, body []byte) (string, error) {
	if err := s.reserve(); err != nil {
		return "", err
	}
	return s.objects.Replace(ctx, name, tag, body)
}
func (s discoveryStorage) ListNoticePage(ctx context.Context, start string) (ocistorage.NoticePage, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.NoticePage{}, err
	}
	return s.objects.ListNoticePage(ctx, start)
}
