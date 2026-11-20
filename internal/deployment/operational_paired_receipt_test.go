package deployment

import (
	"context"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

// State strings exercise reporting only, never verification or persistence.
func TestPairedReceiptProjectionPreservesCompletedVerification(t *testing.T) {
	for _, tc := range []struct {
		d, o                 string
		fail                 bool
		diagnostic, operator bool
	}{
		{"", "", true, false, false},
		{"API_ACCEPTED", "", true, true, false},
		{"API_ACCEPTED", "API_ACCEPTED", true, true, true},
		{"API_ACCEPTED", "API_ACCEPTED", false, true, true},
		{"REVIEW", "", false, false, false},
		{"API_ACCEPTED", "REVIEW", false, true, false},
	} {
		var failure error
		if tc.fail {
			failure = &reconciliation.SetupConfirmationError{Code: "write_failed"}
		}
		var result OperationalResult
		err := pairedReceiptOutcome(context.Background(), tc.d, tc.o, failure, &result)
		if (err == nil) != (tc.diagnostic && tc.operator && !tc.fail) || receiptVerified(t, result.Diagnostic) != tc.diagnostic {
			t.Fatal("incorrect paired outcome", err)
		}
		if result.Operator != nil && receiptVerified(t, result.Operator) != tc.operator {
			t.Fatal("operator verification misreported")
		}
		if tc.operator && result.Operator == nil {
			t.Fatal("verified operator omitted")
		}
		requireReceiptPrivacy(t, result)
	}
}

func TestPairedReceiptFailureCannotFallBackToSending(t *testing.T) {
	called := false
	ops := operations{pairedReceipts: func(context.Context, *reconciliation.Lease) (reconciliation.SetupConfirmationResult, error) {
		called = true
		return reconciliation.SetupConfirmationResult{}, &reconciliation.SetupConfirmationError{Code: "read_failed"}
	}, diagnostic: func(context.Context, *reconciliation.Lease) (reconciliation.DiagnosticResult, error) {
		t.Fatal("confirmation fell back to sender")
		return reconciliation.DiagnosticResult{}, nil
	}}
	var result OperationalResult
	if err := ops.test(context.Background(), nil, &result); err == nil || !called || result.Diagnostic == nil || result.Diagnostic.Code != "read_failed" {
		t.Fatal("failure lost", err)
	}
}
