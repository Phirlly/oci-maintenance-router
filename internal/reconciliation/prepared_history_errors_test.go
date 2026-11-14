package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparedHistoryStoreControlsBackendErrorsWithoutConfirmation(t *testing.T) {
	for _, tt := range []struct {
		err       error
		status    int
		uncertain bool
	}{
		{errors.New("private details"), 0, true}, {&BaselineStorageError{Code: "private details", HTTPStatus: 503}, 0, true},
		{&ocistorage.Error{Code: "private details", Status: 503, WriteUncertain: true}, 503, true},
		{fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}), 0, true},
		{(*ocistorage.Error)(nil), 0, true}, {&ocistorage.Error{Code: ocistorage.Authentication}, 0, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}, 403, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 429, WriteUncertain: true}, 429, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 404}, 404, false},
	} {
		_, _, first, _ := preparedStoreFixture(t)
		backend := &preparedBackend{create: func(context.Context, string, []byte) (string, error) { return "v1", nil }}
		s := preparedStoreFor(t, backend, first.EnvironmentID(), "installation-1")
		old, err := s.Create(leaseBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		next := advancePrepared(t, old.History(), Baseline{}, first)
		calls := 0
		backend.get = func(context.Context, string) (ocistorage.Object, error) { calls++; return ocistorage.Object{}, tt.err }
		backend.create = func(context.Context, string, []byte) (string, error) { calls++; return "", tt.err }
		backend.replace = func(context.Context, string, string, []byte) (string, error) { calls++; return "", tt.err }
		for i, call := range []func() (StoredPreparedHistory, error){func() (StoredPreparedHistory, error) { return s.Load(leaseBudget(t)) }, func() (StoredPreparedHistory, error) { return s.Create(leaseBudget(t)) }, func() (StoredPreparedHistory, error) { return s.Save(leaseBudget(t), old, next) }} {
			before := calls
			got, err := call()
			code := "read_failed"
			if i != 0 {
				code = "write_failed"
			}
			preparedStoredFailure(t, got, err, code, tt.status, i != 0 && tt.uncertain)
			if calls != before+1 || strings.Contains(err.Error(), "private") || errors.Unwrap(err) != nil {
				t.Fatal("raw error or extra I/O escaped")
			}
		}
	}
}
