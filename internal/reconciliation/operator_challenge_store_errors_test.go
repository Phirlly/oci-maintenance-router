package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"oci-maintenance-router/internal/ocistorage"
	"strings"
	"testing"
)

func TestOperatorChallengeStorageControlsErrorsWithoutBlindConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		status    int
		uncertain bool
	}{
		{"raw", errors.New("private details"), 0, true},
		{"forged domain", &OperatorChallengeStorageError{Code: "private details", HTTPStatus: 503}, 0, true},
		{"unknown code", &ocistorage.Error{Code: "private details", Status: 503, WriteUncertain: true}, 503, true},
		{"wrapped", fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}), 0, true},
		{"joined", errors.Join(errors.New("private"), &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}), 0, true},
		{"typed nil", (*ocistorage.Error)(nil), 0, true},
		{"auth", &ocistorage.Error{Code: ocistorage.Authentication}, 0, false},
		{"forbidden", &ocistorage.Error{Code: ocistorage.Service, Status: 403, WriteUncertain: true}, 403, false},
		{"limited", &ocistorage.Error{Code: ocistorage.Service, Status: 429, WriteUncertain: true}, 429, false},
		{"invalid status", &ocistorage.Error{Code: ocistorage.InvalidRequest, Status: 999}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, operation := range []string{"load", "create", "save"} {
				s, objects, b := operatorStoreFixture(t)
				var old StoredOperatorChallenge
				if operation == "save" {
					old = operatorSaved(t, s)
				}
				calls := 0
				b.create = func(context.Context, string, []byte) (string, error) { calls++; return "", tc.err }
				b.replace = func(context.Context, string, string, []byte) (string, error) { calls++; return "", tc.err }
				if operation == "load" {
					b.get = func(context.Context, string) (ocistorage.Object, error) { calls++; return ocistorage.Object{}, tc.err }
				}
				var got StoredOperatorChallenge
				var err error
				code := "write_failed"
				uncertain := tc.uncertain
				switch operation {
				case "load":
					got, err = s.Load(leaseBudget(t), "operator-test-1")
					code = "read_failed"
					uncertain = false
				case "create":
					got, err = s.Create(leaseBudget(t), operatorChallengeFixture(t))
				case "save":
					got, err = s.Save(leaseBudget(t), old, operatorOutcome(t, old, 200))
				}
				operatorStoredFailure(t, got, err, code, tc.status, uncertain)
				expectedReads := 1
				// Create reads before writing; the save fixture made its one GET.
				reads := 0
				for _, call := range objects.calls {
					if call.method == "GET" {
						reads++
					}
				}
				if calls != 1 || strings.Contains(err.Error(), "private") || errors.Unwrap(err) != nil || (operation != "load" && reads != expectedReads) {
					t.Fatal("error leaked or unauthorized confirmation occurred")
				}
			}
		})
	}
}
