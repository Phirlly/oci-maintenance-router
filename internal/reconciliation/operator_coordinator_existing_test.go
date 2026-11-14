package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorExistingClaimsNeverReadChannel(t *testing.T) {
	for _, state := range []OperatorChallengeState{OperatorPublishing, OperatorAccepted, OperatorReview} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprint(state, "/", expired), func(t *testing.T) {
				f := operatorCoordinatorFixtureFor(t)
				original := f.seed(t, state, expired)
				got, err := f.coordinator.Run(leaseBudget(t), f.lease)
				if err != nil || got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 {
					t.Fatal("existing claim caused effects", err)
				}
				if state == OperatorPublishing && expired {
					s := got.Record().Snapshot()
					if s.State != OperatorReview || s.Reason != "expired" || !s.Ambiguous {
						t.Fatal("expired claim not retained for review")
					}
				} else {
					a, _ := original.Encode()
					b, _ := got.Record().Encode()
					if !bytes.Equal(a, b) {
						t.Fatal("existing state changed")
					}
				}
			})
		}
	}
}

func TestOperatorCoordinatorRejectsChangedIntentWithoutEffects(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	original := f.seed(t, OperatorPublishing, true)
	operatorReconfigure(t, f, func(c map[string]any) { c["operatorTest"].(map[string]any)["endpoint"] = "Other@example.invalid" })
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	operatorRunFailure(t, err, "identity_conflict")
	a, _ := original.Encode()
	b, _ := f.saved(t).Encode()
	if got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 || !bytes.Equal(a, b) {
		t.Fatal("changed intent mutated or sent")
	}
}

func TestOperatorCoordinatorDoesNotInferAbsenceFromOther404Errors(t *testing.T) {
	for _, failure := range []error{errors.New("private"), &ocistorage.Error{Code: ocistorage.InvalidResponse, Status: 404}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &OperatorChallengeStorageError{Code: "read_failed", HTTPStatus: 404}} {
		f := operatorCoordinatorFixtureFor(t)
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if c.method == "GET" && c.name == operatorRunObjectName {
				return failure
			}
			return nil
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		operatorRunFailure(t, err, "read_failed")
		if got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 || preparationCalls(f.objects, "HISTORY", "") != 0 {
			t.Fatal("untrusted404 started new request")
		}
	}
}

func TestOperatorCoordinatorRetainedHistoryPreventsRecreation(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	f.objects.history = func(context.Context, string) (bool, error) { return false, nil }
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	operatorRunFailure(t, err, "history_retained")
	if got.Dispatched() || f.sends.Load() != 0 || preparationCalls(f.objects, "CREATE", operatorRunObjectName) != 0 {
		t.Fatal("retained request reset")
	}
}
