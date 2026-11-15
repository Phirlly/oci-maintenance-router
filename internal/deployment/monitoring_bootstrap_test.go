package deployment

import (
	"encoding/json"
	"testing"
)

func TestMetricsBootstrapPreservesLegacyAndBindsDeploymentCompartment(t *testing.T) {
	s, _ := fixture(t)
	for _, tc := range []struct {
		name, compartment string
		valid             bool
	}{
		{"legacy", "", true}, {"compartment", "ocid1.compartment.oc1..router", true},
		{"root", s.TenancyID, true}, {"foreign-root", "ocid1.tenancy.oc1..foreign", false},
		{"invalid", "not-a-compartment", false}, {"wrong-realm", "ocid1.compartment.oc2..router", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(s)
			var value map[string]any
			_ = json.Unmarshal(raw, &value)
			if tc.compartment != "" {
				value["metricsCompartmentId"] = tc.compartment
			}
			raw, _ = json.Marshal(value)
			_, err := parseSettings(string(raw))
			if (err == nil) != tc.valid {
				t.Fatalf("bootstrap validity=%v want=%v", err == nil, tc.valid)
			}
		})
	}
}
