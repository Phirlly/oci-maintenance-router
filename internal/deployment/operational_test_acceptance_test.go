package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFunctionTestCheckRequiresEverySavedAcceptanceWithoutResending(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		diagnostic, operator   int
		withOperator, accepted bool
	}{
		{"diagnostic accepted", 202, 0, false, true},
		{"both accepted", 202, 200, true, true},
		{"operator accepted202", 202, 202, true, true},
		{"diagnostic review", 400, 200, true, false},
		{"diagnostic active", 0, 200, true, false},
		{"operator review", 202, 400, true, false},
		{"operator active", 202, 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, tc.withOperator)
			diagnosticCode := retainDiagnostic(t, s, objects, environment, tc.diagnostic)
			operatorCode := "OMR-OP-not-present"
			if tc.withOperator {
				operatorCode = retainOperator(t, objects, environment, tc.operator)
			}
			before := objects.version
			for i := 0; i < 2; i++ {
				got := invokeOperational(t, h, invocation(s))
				if (got.Status == "operational-verified") != tc.accepted || got.Diagnostic == nil || got.Diagnostic.Dispatched || got.Diagnostic.ObservedStatus != 0 || got.ScanCompleted || receiptVerified(t, got.Diagnostic) || receiptVerified(t, got.Operator) {
					t.Fatalf("incorrect retained TEST result: %+v", got)
				}
				if tc.diagnostic != 202 && got.Operator != nil {
					t.Fatal("operator ran after diagnostic was not accepted")
				}
				if tc.withOperator && tc.diagnostic == 202 && (got.Operator == nil || got.Operator.Dispatched || got.Operator.ObservedStatus != 0) {
					t.Fatal("operator retained outcome missing or reported as new send")
				}
				body, _ := json.Marshal(got)
				for _, private := range []string{diagnosticCode, operatorCode, "operator@example.invalid", "vaultsecret", "OwnerToken", "Payload"} {
					if strings.Contains(string(body), private) {
						t.Fatal("operational response exposed private state")
					}
				}
			}
			if principal.reads != 0 || objects.version != before+4 {
				t.Fatalf("retained test accessed credentials or changed history: credential calls=%d versions=%d/%d", principal.reads, before, objects.version)
			}
		})
	}
}

func TestFunctionTestReconcileDoesNotRunConfiguredDiagnostics(t *testing.T) {
	h, _, objects, _, principal := retainedTestFixture(t, true)
	got := invokeOperational(t, h, `{"operation":"reconcile"}`)
	if got.Status != "idle" || got.Diagnostic != nil || got.Operator != nil || got.ScanCompleted || principal.reads != 0 || len(objects.objects) != 1 {
		t.Fatalf("ordinary reconcile ran a test: %+v", got)
	}
}
