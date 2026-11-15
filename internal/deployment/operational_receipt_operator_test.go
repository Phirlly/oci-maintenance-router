package deployment

import (
	"strings"
	"testing"
)

func TestFunctionRequiresAllConfiguredReceiptCodesBeforeAnyTestWork(t *testing.T) {
	for _, supplied := range []string{"diagnostic", "operator"} {
		t.Run(supplied, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, true)
			diagnostic := retainDiagnostic(t, s, objects, environment, 202)
			operator := retainOperator(t, objects, environment, 202)
			if supplied == "diagnostic" {
				operator = ""
			} else {
				diagnostic = ""
			}
			s = configureReceipts(t, h, s, objects, diagnostic, operator)
			history, before := retainedReceiptHistory(objects), objects.version
			got := invokeOperational(t, h, invocation(s))
			if got.Status != "not-ready" || got.Code != "operation_configuration" || got.Diagnostic != nil || got.Operator != nil || principal.reads != 0 {
				t.Fatalf("incomplete receipt input allowed test work: %+v", got)
			}
			requireReceiptReadOnly(t, objects, history, before)
		})
	}
}

func TestFunctionOperatorReceiptRequiresSavedEvidenceAndFreshChannel(t *testing.T) {
	for _, kind := range []string{"wrong-code", "missing", "review", "channel-read", "diagnostic-failure"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, true)
			diagnostic := retainDiagnostic(t, s, objects, environment, 202)
			operator := "OMR-OP-" + strings.Repeat("c", 32)
			if kind != "missing" {
				status := 202
				if kind == "review" {
					status = 400
				}
				operator = retainOperator(t, objects, environment, status)
			}
			if kind == "wrong-code" {
				operator = "OMR-OP-" + strings.Repeat("0", 32)
			} else if kind == "diagnostic-failure" {
				diagnostic = "OMR-" + strings.Repeat("0", 32)
			}
			s = configureReceipts(t, h, s, objects, diagnostic, operator)
			history, before := retainedReceiptHistory(objects), objects.version
			got := invokeOperational(t, h, invocation(s))
			if got.Status != "not-ready" || got.Diagnostic == nil || got.Diagnostic.Dispatched || got.Code == "" || got.CleanupCode != "" || got.ScanCompleted || got.BaselineSelected {
				t.Fatalf("operator confirmation failure was concealed: %+v", got)
			}
			if kind == "diagnostic-failure" {
				if got.Operator != nil || receiptVerified(t, got.Diagnostic) {
					t.Fatal("operator ran after diagnostic failure")
				}
			} else if !receiptVerified(t, got.Diagnostic) || got.Operator == nil || got.Operator.Dispatched || got.Operator.ObservedStatus != 0 || receiptVerified(t, got.Operator) || got.Operator.Code != got.Code {
				t.Fatal("partial receipt outcome lost or publication reported")
			}
			// Real client composition stops at the deliberately failing signer;
			// no network dispatch or positive managed-channel proof is involved.
			if (principal.reads > 0) != (kind == "channel-read") || kind == "channel-read" && got.Code != "channel_read" {
				t.Fatal("receipt verification skipped saved evidence or current channel read")
			}
			requireReceiptReadOnly(t, objects, history, before)
			requireReceiptPrivacy(t, got, diagnostic, operator)
		})
	}
}
