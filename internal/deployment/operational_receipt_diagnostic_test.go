package deployment

import (
	"strings"
	"testing"
)

func TestFunctionConfirmsSavedDiagnosticWithoutResending(t *testing.T) {
	h, s, objects, environment, principal := retainedTestFixture(t, false)
	code := retainDiagnostic(t, s, objects, environment, 202)
	s = configureReceipts(t, h, s, objects, code, "")
	history := retainedReceiptHistory(objects)
	for range 2 {
		before := objects.version
		got := invokeOperational(t, h, invocation(s))
		if got.Status != "operational-verified" || got.Diagnostic == nil || got.Diagnostic.State != "API_ACCEPTED" || !receiptVerified(t, got.Diagnostic) || got.Diagnostic.Dispatched || got.Diagnostic.ObservedStatus != 0 || got.Operator != nil || got.Code != "" || got.CleanupCode != "" || got.BaselineSelected || got.ScanCompleted {
			t.Fatalf("saved diagnostic was not confirmed: %+v", got)
		}
		if principal.reads != 0 {
			t.Fatal("confirmation accessed credentials")
		}
		requireReceiptReadOnly(t, objects, history, before)
		requireReceiptPrivacy(t, got, code)
	}
}

func TestFunctionRejectsUnverifiedDiagnosticWithoutFallback(t *testing.T) {
	for _, kind := range []string{"wrong-code", "missing", "deleted", "corrupt", "review", "active", "secret", "request", "release", "catalog"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, false)
			status := 202
			if kind == "review" {
				status = 400
			} else if kind == "active" {
				status = 0
			}
			code := "OMR-" + strings.Repeat("c", 32)
			if kind != "missing" {
				code = retainDiagnostic(t, s, objects, environment, status)
			}
			if kind == "wrong-code" {
				code = "OMR-" + strings.Repeat("0", 32)
			}
			for name, object := range objects.objects {
				if strings.HasPrefix(name, "control/diagnostics/") {
					if kind == "deleted" {
						delete(objects.objects, name)
					} else if kind == "corrupt" {
						object.Body = []byte("not a record")
						objects.objects[name] = object
					}
				}
			}
			if kind == "release" {
				s.ReleaseSHA256 = strings.Repeat("e", 64)
			} else if kind == "catalog" {
				objects.pinned.catalog = append(objects.pinned.catalog, '\n')
				s.CatalogSHA256 = digestFixture(objects.pinned.catalog)
			}
			s = configureReceipts(t, h, s, objects, code, "")
			s = configureOperational(t, h, s, objects, func(v map[string]any) {
				d := v["diagnostic"].(map[string]any)
				if kind == "secret" {
					d["secretId"] = "ocid1.vaultsecret.oc1.iad.other"
				} else if kind == "request" {
					d["requestId"] = "test-2"
				}
			})
			history, before := retainedReceiptHistory(objects), objects.version
			got := invokeOperational(t, h, invocation(s))
			if got.Status != "not-ready" || got.Diagnostic == nil || receiptVerified(t, got.Diagnostic) || got.Diagnostic.Dispatched || got.Diagnostic.ObservedStatus != 0 || got.Code == "" || got.Diagnostic.Code != got.Code || got.Operator != nil || principal.reads != 0 {
				t.Fatalf("unverified diagnostic did not fail closed: %+v", got)
			}
			requireReceiptReadOnly(t, objects, history, before)
			requireReceiptPrivacy(t, got, code)
		})
	}
}
