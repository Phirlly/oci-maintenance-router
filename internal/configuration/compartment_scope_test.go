package configuration

import (
	"reflect"
	"slices"
	"testing"
)

func TestAcceptExplicitRootCompartment(t *testing.T) {
	for _, root := range []string{
		"ocid1.tenancy.oc1..example",
		"ocid1.tenancy.oc1...another",
		"ocid1.tenancy.oc1..future.another",
	} {
		for _, scope := range [][]string{{root}, {root, "ocid1.compartment.oc1..child"}} {
			t.Run(root+"/"+scope[len(scope)-1], func(t *testing.T) {
				value := settings(t)
				value["tenancyId"], value["compartmentIds"] = root, scope
				got := decoded(t, value)
				if !reflect.DeepEqual(got.CompartmentIDs, scope) {
					t.Fatalf("selected scope changed: %v", got.CompartmentIDs)
				}
			})
		}
	}
}

func TestRejectInvalidRootCompartmentScope(t *testing.T) {
	root := "ocid1.tenancy.oc1..example"
	for _, tc := range []struct {
		name, tenancy, field string
		scope                []string
	}{
		{"foreign root", root, "compartmentIds", []string{"ocid1.tenancy.oc1..another"}},
		{"duplicate root", root, "compartmentIds", []string{root, root}},
		{"different spelling is not exact", root, "compartmentIds", []string{"ocid1.tenancy.oc1...example"}},
		{"invalid matching tenancy", "ocid1.tenancy.oc2..example", "tenancyId", []string{"ocid1.tenancy.oc2..example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := settings(t)
			value["tenancyId"], value["compartmentIds"] = tc.tenancy, tc.scope
			wantError(t, encoded(t, value), tc.field, "invalid")
		})
	}
}

func TestAddingRootChangesRoutingGeneration(t *testing.T) {
	value := settings(t)
	before := generation(t, decoded(t, value))
	value["compartmentIds"] = []string{"ocid1.compartment.oc1..example", value["tenancyId"].(string)}
	got := decoded(t, value)
	after := generation(t, got)
	if after == before {
		t.Fatal("adding root did not change routing generation")
	}
	slices.Reverse(got.CompartmentIDs)
	if generation(t, got) != after {
		t.Fatal("root selection order changed routing generation")
	}
}
