package deployment

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/reconciliation"
)

func TestReceiptConfirmationStillRequiresFinalGuardsAndCleanup(t *testing.T) {
	for _, kind := range []string{"current", "ownership", "cleanup", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, false)
			code := retainDiagnostic(t, s, objects, environment, 202)
			s = configureReceipts(t, h, s, objects, code, "")
			history := retainedReceiptHistory(objects)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
				ops, err := composeOperations(s, store, p, operation)
				verify := ops.diagnosticReceipt
				ops.diagnosticReceipt = func(work context.Context, lease *reconciliation.Lease) (reconciliation.DiagnosticReceipt, error) {
					if kind == "canceled" {
						cancel()
					}
					receipt, err := verify(work, lease)
					switch kind {
					case "current":
						objects.pinned.config = []byte("changed after confirmation")
					case "ownership":
						if err := lease.Release(work); err != nil {
							t.Fatal(err)
						}
					case "cleanup":
						objects.before = func(_ context.Context, method, _ string) error {
							if method == "REPLACE" {
								return errors.New("private cleanup failure")
							}
							return nil
						}
					}
					return receipt, err
				}
				return ops, err
			}
			got := h.run(withInvocationHeaders(ctx, nil), strings.NewReader(invocation(s))).operational
			want := map[string]string{"current": "configuration_changed", "ownership": "ownership_failed", "cleanup": "", "canceled": "canceled"}[kind]
			if got == nil || got.Status != "not-ready" || got.Code != want || got.Diagnostic == nil || receiptVerified(t, got.Diagnostic) != (kind != "canceled") || got.Diagnostic.Dispatched || got.ScanCompleted || principal.reads != 0 {
				t.Fatalf("receipt bypassed final guards: %+v", got)
			}
			if (got.CleanupCode == "release_failed") != (kind == "cleanup") || !reflect.DeepEqual(retainedReceiptHistory(objects), history) {
				t.Fatal("cleanup result or receipt history changed")
			}
			requireReceiptPrivacy(t, *got, code)
		})
	}
}

func TestReceiptInputsDoNotAuthorizeOrdinaryReconcile(t *testing.T) {
	h, s, objects, _, principal := retainedTestFixture(t, true)
	configureReceipts(t, h, s, objects, "OMR-"+strings.Repeat("a", 32), "OMR-OP-"+strings.Repeat("b", 32))
	got := invokeOperational(t, h, `{"operation":"reconcile"}`)
	if got.Status != "idle" || got.Diagnostic != nil || got.Operator != nil || got.ScanCompleted || principal.reads != 0 || len(objects.objects) != 1 {
		t.Fatal("receipt configuration changed ordinary TEST reconcile")
	}
}
