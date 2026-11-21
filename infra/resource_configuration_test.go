package infra_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
)

const computeService = "Oracle Cloud Infrastructure Compute"

func selectedCatalog() map[string]any {
	c := catalogFixture()
	c["resourceServices"] = map[string][]string{
		"mysqldbsystem": {historicalMySQLService, currentMySQLService, "Oracle HeatWave MySQL"},
		"instance":      {computeService},
	}
	return c
}

func renderSelected(t *testing.T, s map[string]any) (map[string]string, string, error) {
	t.Helper()
	return renderWithCatalog(t, s, "b1-"+strings.Repeat("a", 64), selectedCatalog(), "ocid1.onssubscription.oc1.iad.operator")
}

func TestSelectedConfigurationMatchesRuntimeForAllModes(t *testing.T) {
	var independentCatalog string
	for _, types := range [][]string{{"mysqldbsystem"}, {"instance"}, {"mysqldbsystem", "instance"}} {
		for _, mode := range []string{"PREVIEW", "PAUSED", "LIVE", "SEND_TEST", "CONFIRM"} {
			t.Run(strings.Join(types, "+")+"/"+mode, func(t *testing.T) {
				action, runtimeMode := "NONE", mode
				if mode == "SEND_TEST" || mode == "CONFIRM" {
					action, runtimeMode = mode, "TEST"
				}
				s := setupSettings(runtimeMode, action, true)
				s["resource_types"] = types
				if action == "NONE" {
					s["setup_attempt"], s["test_run"], s["operator_test_run"] = -1, -1, -1
				}
				if action != "CONFIRM" {
					s["test_receipt_code"], s["operator_receipt_code"] = "stale", "stale"
				}
				values, message, err := renderSelected(t, s)
				if err != nil {
					t.Fatalf("selected render: %s (%v)", message, err)
				}
				catalog, err := configuration.DecodeCatalog([]byte(values["catalog"]))
				if err != nil {
					t.Fatal(err)
				}
				c, err := configuration.Decode([]byte(values["configuration"]), catalog)
				want := slices.Clone(types)
				slices.Sort(want)
				if err != nil || c.SchemaVersion != 9 || c.Mode != runtimeMode || !reflect.DeepEqual(c.ResourceTypes, want) || c.Setup == nil || c.Setup.Action != action || c.MySQLServices != nil {
					t.Fatalf("wrong selected runtime contract: %+v (%v)", c, err)
				}
				if catalog.SchemaVersion != 1 || len(catalog.ResourceServices) != 2 || catalog.MySQLServices != nil {
					t.Fatal("incomplete independent catalog")
				}
				if independentCatalog != "" && values["catalog"] != independentCatalog {
					t.Fatal("selection or mode changed full catalog")
				}
				independentCatalog = values["catalog"]
				if c.InstallationID != "b1-"+strings.Repeat("a", 64) || len(c.RegionMap) != 1 || len(c.CompartmentIDs) != 1 || c.Ownership.Key != "Owner" || len(c.Routes) != 2 {
					t.Fatal("shared routing settings changed")
				}
				if (values["confirmation"] == "true") != (action == "CONFIRM") {
					t.Fatal("wrong confirmation intent")
				}
				if c.Diagnostic != nil || c.OperatorTest != nil || c.OperatorChannel == nil || action == "NONE" && c.Setup.Attempt != 0 {
					t.Fatal("selected saved setup retained legacy or inactive intent")
				}
			})
		}
	}
}

func TestSelectedDiagnosticOnlyRetainsExactLegacyOutput(t *testing.T) {
	for _, action := range []string{"SEND_TEST", "CONFIRM"} {
		s := setupSettings("TEST", action, false)
		before, message, err := renderSetup(t, s)
		if err != nil {
			t.Fatalf("legacy: %s (%v)", message, err)
		}
		s["resource_types"] = []string{"instance"}
		after, message, err := renderSelected(t, s)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("diagnostic-only bytes changed: %s (%v)", message, err)
		}
	}
}
