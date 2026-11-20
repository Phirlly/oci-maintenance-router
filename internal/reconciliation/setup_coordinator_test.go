package reconciliation

import (
	"context"
	"errors"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestSetupCoordinatorSelectsBeforeSendAndRetainsConfirmationOnRetry(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || f.result.Diagnostic.Record().Snapshot().State != DiagnosticAccepted || f.result.Operator.Record().Snapshot().State != OperatorAccepted {
		t.Fatal("combined test failed", err)
	}
	f.configure(t, "CONFIRM", 1)
	confirmed, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || confirmed.Confirmation.Confirmation.ID() == "" {
		t.Fatal("paired confirmation not attached", err)
	}
	s, _ := newSetupCurrentStore(f.objects, f.lease.manager.environmentID, "installation-1")
	head, _, err := s.load(leaseBudget(t))
	if err != nil || head.record.data.Confirmation == nil || head.record.data.Confirmation.ID != confirmed.Confirmation.Confirmation.ID() {
		t.Fatal("saved pointer lacks confirmation", err)
	}
	f.configure(t, "SEND_TEST", 1)
	got, err := f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil || got.Diagnostic.Dispatched() || got.Operator.Dispatched() || f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("retry resent consumed tests", err)
	}
	after, _, err := s.load(leaseBudget(t))
	if err != nil || !sameSetupCurrent(head.record, after.record) {
		t.Fatal("retry discarded saved confirmation", err)
	}
}

func TestSetupCoordinatorNewFailedAttemptCannotRetainOlderConfirmation(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	var err error
	f.result, err = f.coordinator.Run(leaseBudget(t), f.lease)
	if err != nil {
		t.Fatal(err)
	}
	f.configure(t, "CONFIRM", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err != nil {
		t.Fatal(err)
	}
	f.configure(t, "SEND_TEST", 2)
	f.coordinator.diagnostic.current = func(context.Context, string) (diagnosticCredential, error) { return nil, errors.New("private failure") }
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err == nil {
		t.Fatal("credential failure accepted")
	}
	s, _ := newSetupCurrentStore(f.objects, f.lease.manager.environmentID, "installation-1")
	head, _, err := s.load(leaseBudget(t))
	if err != nil || head.record.data.Attempt != 2 || head.record.data.Confirmation != nil {
		t.Fatal("newer failed intent retained older authority", err)
	}
	if f.diagnosticSends != 1 || f.operatorSends != 1 {
		t.Fatal("failed new attempt dispatched")
	}
	f.configure(t, "SEND_TEST", 1)
	if _, err = f.coordinator.Run(leaseBudget(t), f.lease); err == nil {
		t.Fatal("old number restored authority")
	}
}

func TestSetupCoordinatorFailedSelectionDoesNotReadCredentialsOrSend(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	f.objects.before = func(_ context.Context, c discoveryCall) error {
		if c.method == "CREATE" && c.name == "control/setup-current/installation-1.json" {
			return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
		}
		return nil
	}
	f.coordinator.diagnostic.current = func(context.Context, string) (diagnosticCredential, error) {
		t.Fatal("credential read before selected attempt")
		return nil, nil
	}
	if _, err := f.coordinator.Run(leaseBudget(t), f.lease); err == nil || f.diagnosticSends != 0 || f.operatorSends != 0 {
		t.Fatal("failed selection dispatched", err)
	}
}
