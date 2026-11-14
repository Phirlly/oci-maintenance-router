package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticCoordinatorExistingClaimsNeverReadCredentials(t *testing.T) {
	for _, state := range []DiagnosticState{DiagnosticSending, DiagnosticAccepted, DiagnosticReview} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprint(state, "/", expired), func(t *testing.T) {
				f := coordinatorFixture(t)
				original := f.seed(t, state, expired)
				got, err := f.coordinator.Run(leaseBudget(t), f.lease)
				if err != nil || got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 {
					t.Fatal("existing claim caused effects", err)
				}
				if state == DiagnosticSending && expired {
					s := got.Record().Snapshot()
					if s.State != DiagnosticReview || s.Reason != "expired" || !s.Ambiguous || s.SecretVersion != 9 {
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

func TestDiagnosticCoordinatorRejectsChangedIntentWithoutEffects(t *testing.T) {
	f := coordinatorFixture(t)
	original := f.seed(t, DiagnosticSending, true)
	diagnosticReconfigure(t, f, func(c map[string]any) { c["pagerDutyRegion"] = "EU" })
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	diagnosticRunFailure(t, err, "identity_conflict")
	a, _ := original.Encode()
	b, _ := f.saved(t).Encode()
	if got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 || !bytes.Equal(a, b) {
		t.Fatal("changed intent mutated or sent")
	}
}

func TestDiagnosticCoordinatorDoesNotInferAbsenceFromOther404Errors(t *testing.T) {
	for _, failure := range []error{errors.New("private"), &ocistorage.Error{Code: ocistorage.InvalidResponse, Status: 404}, fmt.Errorf("private: %w", &ocistorage.Error{Code: ocistorage.Service, Status: 404}), &DiagnosticStorageError{Code: "read_failed", HTTPStatus: 404}} {
		f := coordinatorFixture(t)
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if c.method == "GET" && c.name == diagnosticObjectName {
				return failure
			}
			return nil
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		diagnosticRunFailure(t, err, "read_failed")
		if got.Dispatched() || f.reads.Load() != 0 || f.sends.Load() != 0 || preparationCalls(f.objects, "HISTORY", "") != 0 {
			t.Fatal("untrusted404 started new request")
		}
	}
}

func TestDiagnosticCoordinatorRetainedHistoryPreventsRecreation(t *testing.T) {
	f := coordinatorFixture(t)
	f.objects.history = func(context.Context, string) (bool, error) { return false, nil }
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	diagnosticRunFailure(t, err, "history_retained")
	if got.Dispatched() || f.sends.Load() != 0 || preparationCalls(f.objects, "CREATE", diagnosticObjectName) != 0 {
		t.Fatal("retained request reset")
	}
}
