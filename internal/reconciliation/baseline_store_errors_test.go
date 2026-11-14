package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreDoesNotTrustRawOrWrappedErrors(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	for _, tt := range []struct {
		err       error
		status    int
		uncertain bool
	}{
		{errors.New("private backend details"), 0, true},
		{&BaselineStorageError{Code: "private backend details", HTTPStatus: 503}, 0, true},
		{&ocistorage.Error{Code: "private backend details", Status: 503, WriteUncertain: true}, 503, true},
		{fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}), 0, true},
		{errors.Join(errors.New("private"), &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}), 0, true},
		{(*ocistorage.Error)(nil), 0, true},
		{&ocistorage.Error{Code: ocistorage.Authentication}, 0, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}, 403, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 429, WriteUncertain: true}, 429, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 404}, 404, false},
		{&ocistorage.Error{Code: ocistorage.Service, Status: 700}, 0, false},
		{&ocistorage.Error{Code: ocistorage.Conflict, Status: 400}, 400, false},
	} {
		for _, create := range []bool{false, true} {
			calls := 0
			backend := &baselineBackend{
				get:    func(context.Context, string) (ocistorage.Object, error) { calls++; return ocistorage.Object{}, tt.err },
				create: func(context.Context, string, []byte) (string, error) { calls++; return "", tt.err },
			}
			store := baselineStoreFor(t, backend, b.EnvironmentID())
			var got Baseline
			var err error
			code := "read_failed"
			if create {
				code = "write_failed"
				got, err = store.Create(leaseBudget(t), b)
			} else {
				got, err = store.Load(leaseBudget(t), b.ID())
			}
			baselineStoredFailure(t, got, err, code, tt.status, create && tt.uncertain)
			if strings.Contains(err.Error(), "private") || errors.Unwrap(err) != nil || calls != 1 {
				t.Fatal("raw details, cause or extra call escaped")
			}
		}
	}
}
