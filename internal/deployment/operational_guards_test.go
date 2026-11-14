package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-maintenance-router/internal/reconciliation"
)

func TestOperationalGuardsRefuseWorkAndReleaseOnlyTheirOwnLease(t *testing.T) {
	for _, kind := range []string{"acquisition", "current-read", "current-changed", "ownership-lost", "composition", "cleanup"} {
		t.Run(kind, func(t *testing.T) {
			h, s, objects, _ := operationalFixture(t)
			s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
			composed, changed := false, false
			objects.before = func(_ context.Context, method, name string) error {
				if kind == "acquisition" && method == "CREATE" || kind == "current-read" && name == "configuration/router.json" || kind == "cleanup" && method == "REPLACE" {
					return errors.New("private provider detail")
				}
				if kind == "current-changed" && name == "configuration/catalog.json" {
					objects.pinned.catalog = []byte("changed")
				}
				if kind == "ownership-lost" && method == "GET" && !strings.HasPrefix(name, "configuration/") && !changed {
					objects.mu.Lock()
					obj := objects.objects[name]
					obj.ETag = "replacement-owner-version"
					objects.objects[name] = obj
					objects.mu.Unlock()
					changed = true
				}
				return nil
			}
			h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
				composed = true
				if kind == "composition" {
					return operations{}, errors.New("private composition detail")
				}
				return composeOperations(s, store, p, operation)
			}
			got := invokeOperational(t, h, invocation(s))
			want := map[string]string{"acquisition": "ownership_failed", "current-read": "current_configuration_read", "current-changed": "configuration_changed", "ownership-lost": "ownership_failed", "composition": "operation_configuration", "cleanup": ""}[kind]
			if got.Status != "not-ready" || got.Code != want || got.ScanCompleted || composed != (kind == "composition" || kind == "cleanup") {
				t.Fatalf("guard did not stop dependent work: %+v; composed=%v", got, composed)
			}
			if (got.CleanupCode == "release_failed") != (kind == "cleanup" || kind == "ownership-lost") {
				t.Fatalf("cleanup outcome concealed: %+v", got)
			}
			if kind == "ownership-lost" {
				for _, obj := range objects.objects {
					if obj.ETag != "replacement-owner-version" {
						t.Fatal("cleanup overwrote replacement ownership")
					}
				}
			}
		})
	}
}

func TestRetainedTestAcceptanceStillRequiresFinalCurrentGuards(t *testing.T) {
	for _, control := range []string{"configuration", "ownership"} {
		t.Run(control, func(t *testing.T) {
			h, s, objects, environment, principal := retainedTestFixture(t, false)
			retainDiagnostic(t, s, objects, environment, 202)
			h.compose = func(s Settings, store Store, p pinnedInputs, operation string) (operations, error) {
				ops, err := composeOperations(s, store, p, operation)
				run := ops.diagnostic
				ops.diagnostic = func(ctx context.Context, lease *reconciliation.Lease) (reconciliation.DiagnosticResult, error) {
					result, err := run(ctx, lease)
					if control == "configuration" {
						objects.pinned.config = []byte("changed after retained acceptance")
					} else if err := lease.Release(ctx); err != nil {
						t.Fatal(err)
					}
					return result, err
				}
				return ops, err
			}
			got := invokeOperational(t, h, invocation(s))
			want := "configuration_changed"
			if control == "ownership" {
				want = "ownership_failed"
			}
			if got.Status != "not-ready" || got.Code != want || got.Diagnostic == nil || got.Diagnostic.State != "API_ACCEPTED" || got.ScanCompleted || principal.reads != 0 {
				t.Fatalf("retained acceptance bypassed final guard: %+v", got)
			}
		})
	}
}

func TestOwnershipLostDuringCurrentReadsPreventsComposition(t *testing.T) {
	h, s, objects, _ := operationalFixture(t)
	s = configureOperational(t, h, s, objects, func(v map[string]any) { v["mode"] = "PAUSED" })
	objects.before = func(_ context.Context, _, name string) error {
		if name == "configuration/catalog.json" {
			objects.mu.Lock()
			for key, obj := range objects.objects {
				obj.ETag = "another-owner"
				objects.objects[key] = obj
			}
			objects.mu.Unlock()
		}
		return nil
	}
	h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
		t.Fatal("lost owner reached composition")
		return operations{}, nil
	}
	got := invokeOperational(t, h, invocation(s))
	if got.Status != "not-ready" || got.Code != "ownership_failed" || got.CleanupCode != "release_failed" || objects.version != 1 {
		t.Fatalf("ownership check lost: %+v", got)
	}
}
