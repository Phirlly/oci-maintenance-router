package reconciliation

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

func TestResourceSelectionCoordinatorsAdmitWithoutOperations(t *testing.T) {
	f := setupRuntimeFixtureFor(t)
	_, catalogWire, cat := selectionInputs(t)
	location, release := candidateLocation(), strings.Repeat("b", 64)
	source := processingEmptySource()
	channel, vault, clients := operatorClient(t), diagnosticVault(t), processingMySQL(t)
	clients.Compute = processingCompute(t).Compute
	constructors := []struct {
		name, mode, action string
		build              func([]byte, []byte) (bool, error)
	}{
		{"preparation", "PREVIEW", "NONE", func(c, b []byte) (bool, error) {
			v, e := NewBaselinePreparation(c, b, location, source, f.objects)
			return v != nil, e
		}},
		{"selector", "LIVE", "NONE", func(c, b []byte) (bool, error) {
			v, e := NewLiveBaselineSelector(c, b, location, release, f.objects, channel)
			return v != nil, e
		}},
		{"processor", "LIVE", "NONE", func(c, b []byte) (bool, error) {
			v, e := NewMaintenanceProcessor(c, b, location, release, source, f.objects, channel, vault, clients)
			return v != nil, e
		}},
		{"reviewer", "LIVE", "NONE", func(c, b []byte) (bool, error) {
			v, e := NewRecoveryReviewer(c, b, location, source, savedDetails{}, clients)
			return v != nil, e
		}},
		{"setup", "TEST", "SEND_TEST", func(c, b []byte) (bool, error) {
			v, e := NewSetupCoordinator(c, b, location, release, f.objects, channel, vault)
			return v != nil, e
		}},
		{"setup confirmation", "TEST", "CONFIRM", func(c, b []byte) (bool, error) {
			v, e := NewSetupCoordinator(c, b, location, release, f.objects, channel, vault)
			return v != nil, e
		}},
		{"readiness", "LIVE", "NONE", func(c, b []byte) (bool, error) {
			v, e := newSetupReadiness(c, b, location, f.objects, channel)
			return v != nil, e
		}},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			configure := func(version int) []byte {
				return setupRuntimeConfig(t, tc.mode, tc.action, 1, func(v map[string]any) {
					if tc.action == "CONFIRM" {
						s := v["setup"].(map[string]any)
						s["pagerDutyCode"], s["operatorCode"] = "OMR-"+strings.Repeat("a", 32), "OMR-OP-"+strings.Repeat("b", 32)
					}
					if version == 9 {
						v["schemaVersion"], v["resourceTypes"] = 9, []string{"mysqldbsystem", "instance"}
						delete(v, "mysqlServices")
					}
				})
			}
			before := len(f.objects.calls)
			if valid, err := tc.build(configure(8), []byte(preparationCatalog)); !valid || err != nil {
				t.Fatal("control dependencies are invalid", err)
			}
			cfg := configure(9)
			if _, err := configuration.Decode(cfg, cat); err != nil {
				t.Fatal("selected input invalid before admission", err)
			}
			if valid, err := tc.build(cfg, catalogWire); !valid || err != nil {
				t.Fatal("selected-resource coordinator rejected", err)
			}
			if len(f.objects.calls) != before || f.diagnosticSends != 0 || f.operatorSends != 0 {
				t.Fatal("constructor performed operations")
			}
		})
	}
}
