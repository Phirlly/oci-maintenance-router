package infra_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestRenderedCompartmentScopeMatchesRouting(t *testing.T) {
	root, child := "ocid1.tenancy.oc1..example", "ocid1.compartment.oc1..example"
	for _, tc := range []struct {
		name  string
		scope []string
	}{
		{"root only", []string{root}},
		{"child only", []string{child}},
		{"root and child", []string{root, child}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := settingsFixture()
			settings["mysql_compartments"] = tc.scope
			got, output, err := render(t, settings)
			if err != nil {
				t.Fatalf("valid scope rejected: %v %s", err, output)
			}
			var catalog configuration.Catalog
			if err := json.Unmarshal([]byte(got["catalog"]), &catalog); err != nil {
				t.Fatal(err)
			}
			config, err := configuration.Decode([]byte(got["configuration"]), catalog)
			if err != nil {
				t.Fatalf("runtime rejected rendered scope: %v", err)
			}
			wantScope := slices.Clone(tc.scope)
			slices.Sort(wantScope)
			if !reflect.DeepEqual(config.CompartmentIDs, wantScope) || config.Mode != "PREVIEW" {
				t.Fatalf("scope or nonsending mode changed: %+v", config)
			}
			p, err := routing.New([]byte(got["configuration"]), catalog)
			if err != nil {
				t.Fatal(err)
			}
			for _, location := range []string{root, child, "ocid1.compartment.oc1..unselected"} {
				id, platform := "ocid1.mysqldbsystem.oc1.phx.example", "IAAS"
				n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE",
					Services: []string{"Oracle HeatWave MySQL"}, Platform: &platform, Resources: []notices.Resource{{ResourceID: id}}})
				if err != nil {
					t.Fatal(err)
				}
				plan := p.Resolve(n, false, map[string]routing.Lookup{id: {State: routing.Found, Metadata: routing.Metadata{
					ID: &id, CompartmentID: &location, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "billing"}},
				}}})
				if len(plan.Decisions) != 1 {
					t.Fatalf("missing scope decision: %+v", plan)
				}
				decision := plan.Decisions[0]
				if slices.Contains(tc.scope, location) {
					wantGroups := []routing.Group{{DestinationID: "payments", Resources: []routing.Target{{ResourceID: id, Region: "us-phoenix-1"}}}}
					if decision.Disposition != routing.Ready || decision.Reason != "" || decision.DestinationID != "payments" || !reflect.DeepEqual(plan.Groups, wantGroups) {
						t.Fatalf("selected resource lost owner routing: %+v", plan)
					}
				} else if decision.Disposition != routing.OutOfScope || decision.Reason != routing.ReasonCompartmentScope || decision.DestinationID != "" || len(plan.Groups) != 0 {
					t.Fatalf("unselected compartment %s entered routing: %+v", location, plan)
				}
			}
		})
	}
}

func TestTerraformRejectsInvalidRootCompartmentScope(t *testing.T) {
	root := "ocid1.tenancy.oc1..example"
	for _, tc := range []struct {
		name, tenancy, message string
		scope                  []string
	}{
		{"foreign root", root, "Compartments", []string{"ocid1.tenancy.oc1..another"}},
		{"duplicate root", root, "Compartments", []string{root, root}},
		{"nonexact root", root, "Compartments", []string{"ocid1.tenancy.oc1...example"}},
		{"invalid matching tenancy", "ocid1.tenancy.oc2..example", "OC1 tenancy", []string{"ocid1.tenancy.oc2..example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := settingsFixture()
			settings["tenancy_ocid"], settings["mysql_compartments"] = tc.tenancy, tc.scope
			_, output, err := render(t, settings)
			if err == nil || !strings.Contains(output, tc.message) {
				t.Fatalf("wanted %q rejection, got %v: %s", tc.message, err, output)
			}
		})
	}
}
