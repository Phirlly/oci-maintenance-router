package deployment

import "testing"

func TestMetricsUseOnlyTrustedManagedDestination(t *testing.T) {
	s, objects := fixture(t)
	s.MetricsCompartmentID = "ocid1.compartment.oc1..router"
	for _, kind := range []string{"valid", "ordinary-store", "nil-principal", "missing-compartment", "foreign-root", "invalid-installation"} {
		t.Run(kind, func(t *testing.T) {
			settings := s
			var store Store = &managedStore{principal: &testPrincipal{}}
			installation := "installation-1"
			switch kind {
			case "ordinary-store":
				store = objects
			case "nil-principal":
				store = &managedStore{}
			case "missing-compartment":
				settings.MetricsCompartmentID = ""
			case "foreign-root":
				settings.MetricsCompartmentID = "ocid1.tenancy.oc1..foreign"
			case "invalid-installation":
				installation = "untrusted/scope"
			}
			publisher, err := composeMetrics(settings, store, installation)
			if (err == nil) != (kind == "valid") || kind == "valid" && publisher == nil {
				t.Fatal("invalid managed metric binding", err)
			}
		})
	}
}
