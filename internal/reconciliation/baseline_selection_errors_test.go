package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionControlsWriteErrorsWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		err       error
		status    int
		uncertain bool
	}{
		{errors.New("private details"), 0, true},
		{&BaselineStorageError{Code: "private details", HTTPStatus: 503}, 0, true},
		{&ocistorage.Error{Code: "private details", Status: 503, WriteUncertain: true}, 503, true},
		{fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}), 0, true},
		{(*ocistorage.Error)(nil), 0, true},
		{&ocistorage.Error{Code: ocistorage.Authentication}, 0, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}, 403, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 429, WriteUncertain: true}, 429, false},
	} {
		for _, create := range []bool{true, false} {
			s, _, backend, first, second := selectionStoreFixture(t)
			next := selectionFor(t, first)
			var old StoredBaselineSelection
			if !create {
				old = selectionSaved(t, s, first)
				next = advanceSelection(t, old.Selection(), first, second)
			}
			calls := 0
			backend.create = func(context.Context, string, []byte) (string, error) { calls++; return "", tc.err }
			backend.replace = func(context.Context, string, string, []byte) (string, error) { calls++; return "", tc.err }
			var got StoredBaselineSelection
			var err error
			if create {
				got, err = s.Create(leaseBudget(t), next)
			} else {
				got, err = s.Save(leaseBudget(t), old, next)
			}
			selectionStoredFailure(t, got, err, "write_failed", tc.status, tc.uncertain)
			if calls != 1 || strings.Contains(err.Error(), "private") || errors.Unwrap(err) != nil {
				t.Fatal("raw error or retry escaped")
			}
		}
	}
}
