package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
)

// Preserve the original invocation reserve on every dispatch. Recheck current
// controls immediately before creating evidence, including after history reads.
type setupConfirmationStorage struct {
	objects        SetupConfirmationObjects
	reserve, guard func() error
}

// Only the owned pre-dispatch wrapper can identify a guard rejection. A backend
// error with a similar public Code remains untrusted and potentially uncertain.
type setupConfirmationBlocked struct{ failure error }

func (e *setupConfirmationBlocked) Error() string { return "setup confirmation dispatch blocked" }

func (s setupConfirmationStorage) Get(ctx context.Context, name string) (ocistorage.Object, error) {
	if err := s.reserve(); err != nil {
		return ocistorage.Object{}, err
	}
	return s.objects.Get(ctx, name)
}
func (s setupConfirmationStorage) HistoryIsEmpty(ctx context.Context, name string) (bool, error) {
	if err := s.reserve(); err != nil {
		return false, err
	}
	return s.objects.HistoryIsEmpty(ctx, name)
}
func (s setupConfirmationStorage) Create(ctx context.Context, name string, body []byte) (string, error) {
	if err := s.guard(); err != nil {
		return "", &setupConfirmationBlocked{err}
	}
	return s.objects.Create(ctx, name, body)
}
