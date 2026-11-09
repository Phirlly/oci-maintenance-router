package routing_test

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestCompartmentScopeIsExplicit(t *testing.T) {
	for _, root := range []string{"ocid1.tenancy.oc1..example", "ocid1.tenancy.oc1..future.another"} {
		for _, tc := range []struct {
			name            string
			scope           []string
			rootIn, childIn bool
		}{
			{"root only", []string{root}, true, false},
			{"child only", []string{compartment}, false, true},
			{"root and child", []string{root, compartment}, true, true},
		} {
			t.Run(root+"/"+tc.name, func(t *testing.T) {
				data := settings(t, func(value map[string]any) {
					value["tenancyId"], value["compartmentIds"] = root, tc.scope
				})
				p, err := routing.New(data, catalog())
				if err != nil {
					t.Fatal(err)
				}
				for _, location := range []struct {
					id      string
					allowed bool
				}{{root, tc.rootIn}, {compartment, tc.childIn}, {"ocid1.compartment.oc1..unselected", false}} {
					lookup := metadata(db, "payments")
					lookup.Metadata.CompartmentID = text(location.id)
					got := p.Resolve(notice(t, record(db)), false, map[string]routing.Lookup{db: lookup})
					want := routing.Decision{Index: 0, ResourceID: db, Region: "us-ashburn-1", Disposition: routing.OutOfScope, Reason: routing.ReasonCompartmentScope}
					var groups []routing.Group
					if location.allowed {
						want.Disposition, want.Reason, want.DestinationID = routing.Ready, "", "payments"
						groups = []routing.Group{{DestinationID: "payments", Resources: []routing.Target{{ResourceID: db, Region: "us-ashburn-1"}}}}
					}
					if !reflect.DeepEqual(got.Decisions, []routing.Decision{want}) || !reflect.DeepEqual(got.Groups, groups) {
						t.Fatalf("scope %v, metadata %s: %+v", tc.scope, location.id, got)
					}
				}
			})
		}
	}
}

func TestRootScopeKeepsInvalidMetadataAndUnmappedOwnersUnresolved(t *testing.T) {
	root := "ocid1.tenancy.oc1..example"
	data := settings(t, func(value map[string]any) { value["compartmentIds"] = []string{root} })
	p, err := routing.New(data, catalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, location, owner string
		reason                routing.Reason
	}{
		{"foreign root", "ocid1.tenancy.oc1..another", "payments", routing.ReasonCompartment},
		{"nonexact root", "ocid1.tenancy.oc1...example", "payments", routing.ReasonCompartment},
		{"empty compartment", "", "payments", routing.ReasonCompartment},
		{"unmapped owner", root, "unmapped", routing.ReasonOwnerUnmapped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := metadata(db, tc.owner)
			lookup.Metadata.CompartmentID = text(tc.location)
			got := p.Resolve(notice(t, record(db)), false, map[string]routing.Lookup{db: lookup})
			if len(got.Decisions) != 1 || got.Decisions[0].Disposition != routing.Unresolved || got.Decisions[0].Reason != tc.reason || got.Decisions[0].DestinationID != "" || len(got.Groups) != 0 {
				t.Fatalf("unresolved resource produced routing work: %+v", got)
			}
		})
	}
}
