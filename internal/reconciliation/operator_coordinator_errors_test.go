package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocinotifications"
)

func TestOperatorCoordinatorExternalErrorsCannotForgeDiagnostics(t *testing.T) {
	for _, stage := range []string{"current", "storage", "channel"} {
		for _, failure := range []error{errors.New("private target"), &OperatorRunError{Code: "private target"}, &OperatorChallengeStorageError{Code: "private target", HTTPStatus: 999}, (*ocinotifications.Error)(nil)} {
			f := operatorCoordinatorFixtureFor(t)
			want := "channel_read"
			if stage == "channel" {
				f.coordinator.read = func(context.Context) (operatorObservation, error) { return operatorObservation{}, failure }
			} else {
				name := operatorRunObjectName
				want = "read_failed"
				if stage == "current" {
					name = "configuration/router.json"
					want = "configuration_read"
				}
				f.objects.before = func(_ context.Context, c discoveryCall) error {
					if c.name == name {
						return failure
					}
					return nil
				}
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			e := operatorRunFailure(t, err, want)
			if e.HTTPStatus != 0 || strings.Contains(e.Error(), "private") || errors.Unwrap(e) != nil || got.Dispatched() {
				t.Fatal("external diagnostics escaped or authorized publication")
			}
		}
	}
}

func TestOperatorCoordinatorExpiredCallerCannotStart(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	before := len(f.objects.calls)
	got, err := f.coordinator.Run(ctx, f.lease)
	e := operatorRunFailure(t, err, "deadline_exceeded")
	if !errors.Is(e, context.DeadlineExceeded) || got.Dispatched() || len(f.objects.calls) != before {
		t.Fatal("expired caller revived")
	}
}

func TestOperatorCoordinatorChannelStatusIsBounded(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	f.coordinator.read = func(context.Context) (operatorObservation, error) {
		return operatorObservation{}, &ocinotifications.Error{Code: ocinotifications.Service, Status: 999}
	}
	_, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if operatorRunFailure(t, err, "channel_read").HTTPStatus != 0 {
		t.Fatal("invalid status escaped")
	}
}
