package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func configureResourceSelection(t *testing.T, h *Handler, s Settings, objects *operationalStore, mode, action string, kinds ...string) Settings {
	t.Helper()
	var catalog map[string]any
	if err := json.Unmarshal(objects.pinned.catalog, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog["schemaVersion"] = 1
	catalog["resourceServices"] = map[string]any{"mysqldbsystem": catalog["mysqlServices"], "instance": []string{"Oracle Cloud Infrastructure Compute"}}
	delete(catalog, "mysqlServices")
	objects.pinned.catalog, _ = json.Marshal(catalog)
	s.CatalogSHA256 = digest(objects.pinned.catalog)
	return configureOperational(t, h, s, objects, func(v map[string]any) {
		setupConfiguration(mode, action)(v)
		v["schemaVersion"], v["resourceTypes"] = 9, kinds
		delete(v, "mysqlServices")
		v["preparation"] = map[string]any{"initialize": false}
	})
}

func TestResourceSelectionHandlerUsesOperationalModes(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST", "LIVE"} {
		t.Run(mode, func(t *testing.T) {
			h, s, objects, _, principal := retainedTestFixture(t, true)
			action := "NONE"
			if mode == "TEST" {
				action = "SEND_TEST"
			}
			s = configureResourceSelection(t, h, s, objects, mode, action, "instance")
			objects.before = func(_ context.Context, method, name string) error {
				if method == "GET" && (strings.HasPrefix(name, "control/setup-current/") || name == "control/prepared-history/synthetic-installation.json") {
					return errors.New("synthetic guarded read")
				}
				return nil
			}
			got := invokeOperational(t, h, invocation(s))
			if got.ResponseVersion != 2 || got.Mode != mode || got.Code == "configuration_unsupported" || principal.reads != 0 {
				t.Fatal("selected mode bypassed operational guards", got)
			}
			if mode == "PAUSED" {
				if got.Status != "operational-verified" {
					t.Fatal(got)
				}
			} else if got.Status != "not-ready" || got.Code != "read_failed" {
				t.Fatal("real preparation/setup/selection not reached", got)
			}
		})
	}
}

func TestResourceSelectionAutomaticNonLiveRemainsIdle(t *testing.T) {
	for _, mode := range []string{"PREVIEW", "PAUSED", "TEST"} {
		h, s, objects, _, principal := retainedTestFixture(t, true)
		action := "NONE"
		if mode == "TEST" {
			action = "SEND_TEST"
		}
		configureResourceSelection(t, h, s, objects, mode, action, "instance")
		h.compose = func(Settings, Store, pinnedInputs, string) (operations, error) {
			t.Fatal("automatic nonLIVE composed work")
			return operations{}, nil
		}
		before := objects.version
		got := automaticResponse(t, h, operationalContext(t), nil, strings.NewReader(`{"operation":"wake-up"}`))
		if got.Status != "idle" || got.Mode != mode || objects.version != before || principal.reads != 0 {
			t.Fatal("automatic nonLIVE performed work", got)
		}
	}
}

func TestSelectedCompositionUsesCompleteTypeBindingsWithoutIO(t *testing.T) {
	for _, kinds := range [][]string{{"instance"}, {"mysqldbsystem"}, {"mysqldbsystem", "instance"}} {
		h, s, objects, _, principal := retainedTestFixture(t, true)
		s = configureResourceSelection(t, h, s, objects, "LIVE", "NONE", kinds...)
		p, failure := loadPinned(operationalContext(t), s, objects)
		if failure.Status != "" {
			t.Fatal(failure)
		}
		before := objects.version
		ops, err := composeOperations(s, &managedStore{objects, principal}, p, "reconcile")
		if err != nil || ops.process == nil || ops.reportRetry == nil || ops.reportRefresh == nil || principal.reads != 0 || objects.version != before {
			t.Fatal("selected LIVE composition failed", kinds, err)
		}
	}
}
