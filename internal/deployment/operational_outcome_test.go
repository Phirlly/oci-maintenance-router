package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestOperationalErrorsExposeOnlyControlledCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"preparation", &reconciliation.PreparationError{Code: "read_failed"}, "read_failed"},
		{"diagnostic", &reconciliation.DiagnosticRunError{Code: "secret_read"}, "secret_read"},
		{"operator", &reconciliation.OperatorRunError{Code: "channel_read"}, "channel_read"},
		{"selection", &reconciliation.LiveSelectionError{Code: "readiness_required"}, "readiness_required"},
		{"processing", &reconciliation.ProcessingError{Code: "write_failed"}, "write_failed"},
		{"reporting", &reconciliation.ReportingError{Code: "publication_unconfirmed"}, "publication_unconfirmed"},
		{"foreign", errors.New("private provider detail"), "operation_failed"},
		{"invalid-code", &reconciliation.ProcessingError{Code: "https://private.example/credential"}, "operation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, s, _, _ := operationalFixture(t)
			h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
				return operations{initialize: func(context.Context, *reconciliation.Lease) error { return tc.err }}, nil
			}
			got := invokeOperational(t, h, invocation(s))
			if got.Status != "not-ready" || got.Code != tc.want || got.BaselinePrepared || got.ScanCompleted || got.CleanupCode != "" {
				t.Fatalf("unsafe error projection: %+v", got)
			}
		})
	}
}

func TestExpiredOperationReportsDeadlineWithoutProviderDetail(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := operationCode(ctx, errors.New("private provider detail")); got != "deadline_exceeded" {
		t.Fatalf("deadline hidden by provider error: %s", got)
	}
}

func TestTestFailurePreservesConfirmedDiagnosticAndStopsDependentWork(t *testing.T) {
	for _, kind := range []string{"diagnostic", "operator"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, true)
			if kind == "operator" {
				retainDiagnostic(t, s, objects, environment, 202)
			}
			// No retained failed target: the real adapter must reject signing before
			// any network request. This is negative real-client evidence only.
			got := invokeOperational(t, h, invocation(s))
			if got.Status != "not-ready" || got.Diagnostic == nil || got.Diagnostic.Dispatched || principal.reads == 0 || got.ScanCompleted {
				t.Fatalf("failed TEST reported success: %+v", got)
			}
			if kind == "diagnostic" {
				if got.Operator != nil || got.Code != "secret_read" || got.Diagnostic.Code != "secret_read" {
					t.Fatalf("diagnostic failure did not stop operator: %+v", got)
				}
			} else if got.Diagnostic.State != "API_ACCEPTED" || got.Operator == nil || got.Operator.Dispatched || got.Operator.Code == "" || got.Code == "" {
				t.Fatalf("partial saved acceptance lost: %+v", got)
			}
		})
	}
}

func TestCompletedProcessingWithFailedCleanupIsNotSuccessfulScan(t *testing.T) {
	h, _, objects, _ := liveOperationFixture(t)
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		ops := successfulReporting()
		ops.process = func(context.Context, *reconciliation.Lease) (reconciliation.ProcessingSnapshot, error) {
			objects.before = func(_ context.Context, method, _ string) error {
				if method == "REPLACE" {
					return errors.New("private cleanup detail")
				}
				return nil
			}
			return reconciliation.ProcessingSnapshot{Completed: true, Accepted: 2}, nil
		}
		return ops, nil
	}
	got := invokeOperational(t, h, `{"operation":"reconcile"}`)
	if got.Status != "not-ready" || got.ScanCompleted || got.CleanupCode != "release_failed" || got.Processing == nil || got.Processing.Accepted != 2 {
		t.Fatalf("cleanup failure concealed or receipts lost: %+v", got)
	}
}

func TestActualLiveOperationsRequireCurrentServiceEvidence(t *testing.T) {
	for _, operation := range []string{"deployment-check", "reconcile"} {
		t.Run(operation, func(t *testing.T) {
			h, s, _, _ := liveOperationFixture(t)
			body := invocation(s)
			if operation == "reconcile" {
				body = `{"operation":"reconcile"}`
			}
			got := invokeOperational(t, h, body)
			if got.Status != "not-ready" || got.Code == "operation_configuration" || got.Code == "operation_failed" || got.Code == "" || got.BaselineSelected || got.ScanCompleted || got.CleanupCode != "" {
				t.Fatalf("actual coordinator missing or stale receipts authorized work: %+v", got)
			}
		})
	}
}
