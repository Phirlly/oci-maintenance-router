package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReceiptCompositionReturnsVerifiersOnlyAndNoPartialAuthority(t *testing.T) {
	for _, kind := range []string{"diagnostic", "both", "missing-diagnostic", "missing-operator", "release"} {
		t.Run(kind, func(t *testing.T) {
			withOperator := kind != "diagnostic" && kind != "release"
			h, s, objects, _, principal := retainedTestFixture(t, withOperator)
			diagnostic, operator := "OMR-"+strings.Repeat("a", 32), "OMR-OP-"+strings.Repeat("b", 32)
			if kind == "missing-diagnostic" {
				diagnostic = ""
			} else if kind == "missing-operator" {
				operator = ""
			}
			s = configureReceipts(t, h, s, objects, diagnostic, operator)
			p, failure := loadPinned(operationalContext(t), s, objects)
			if failure.Status != "" {
				t.Fatal(failure)
			}
			if kind == "release" {
				s.ReleaseSHA256 = "invalid"
			}
			ops, err := composeOperations(s, &managedStore{objects, principal}, p, "deployment-check")
			valid := kind == "diagnostic" || kind == "both"
			if (err == nil) != valid || (ops.diagnosticReceipt != nil) != (kind == "diagnostic") || (ops.pairedReceipts != nil) != (kind == "both") || ops.diagnostic != nil || ops.operator != nil || ops.initialize != nil || ops.prepare != nil || ops.selectLive != nil || ops.process != nil || principal.reads != 0 || objects.version != 0 {
				t.Fatal("receipt composition returned send or partial authority")
			}
		})
	}
}

func TestReceiptSummaryCannotTurnMissingOrFailedProofIntoConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		err         error
		confirmed   bool
	}{
		{"accepted", "API_ACCEPTED", nil, true},
		{"zero-proof", "", nil, false},
		{"late-error", "API_ACCEPTED", errors.New("private provider detail"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Output projection only; this synthetic state is not verifier authority.
			got, err := receiptOutcome(context.Background(), tc.state, tc.err)
			if (err == nil) != tc.confirmed || receiptVerified(t, got) != tc.confirmed || got.Dispatched || got.ObservedStatus != 0 || got.State != tc.state || (got.Code == "") != tc.confirmed || strings.Contains(got.Code, "private") {
				t.Fatalf("incorrect receipt summary: %+v (%v)", got, err)
			}
		})
	}
}
