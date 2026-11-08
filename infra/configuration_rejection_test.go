package infra_test

import (
	"strings"
	"testing"
)

func TestConfigurationRejectsInvalidCustomerChoices(t *testing.T) {
	cases := []struct {
		name, field string
		value       any
		message     string
	}{
		{"environment", "environment_name", "Bad Name", "Environment name"},
		{"tenancy", "tenancy_ocid", "ocid1.tenancy.oc2..example", "OC1 tenancy"},
		{"region", "region", "unknown", "Deployment region"},
		{"empty regions", "mysql_regions", []string{}, "MySQL regions"},
		{"unknown region", "mysql_regions", []string{"unknown"}, "MySQL regions"},
		{"duplicate region", "mysql_regions", []string{"us-ashburn-1", "us-ashburn-1"}, "MySQL regions"},
		{"empty compartments", "mysql_compartments", []string{}, "MySQL compartments"},
		{"regional compartment", "mysql_compartments", []string{"ocid1.compartment.oc1.iad.example"}, "MySQL compartments"},
		{"tag kind", "ownership_kind", "any", "Ownership tag"},
		{"tag namespace", "ownership_namespace", "has space", "Ownership tag"},
		{"tag key", "ownership_key", "has.period", "Ownership tag"},
		{"no destinations", "destinations", map[string]string{}, "Destinations"},
		{"bad destination", "destinations", map[string]string{"Invalid": "ocid1.vaultsecret.oc1.iad.example"}, "Destinations"},
		{"foreign secret", "destinations", map[string]string{"payments": "ocid1.vaultsecret.oc1.phx.example"}, "Destinations"},
		{"empty routes", "routes", map[string]string{}, "Routes"},
		{"empty owner", "routes", map[string]string{"": "payments"}, "Routes"},
		{"unknown destination", "routes", map[string]string{"payments": "missing"}, "Routes"},
		{"pd region", "pagerduty_region", "APAC", "PagerDuty region"},
		{"variable size", "routes", map[string]string{strings.Repeat("é", 4500): "payments"}, "8192-byte"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := settingsFixture()
			settings[tc.field] = tc.value
			_, output, err := render(t, settings)
			if err == nil || !strings.Contains(output, tc.message) {
				t.Fatalf("want rejection containing %q, got %v: %s", tc.message, err, output)
			}
		})
	}
}
