package infra_test

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func renderReleaseCatalog(t *testing.T, catalog map[string]any) (map[string]string, configuration.Catalog, configuration.Config) {
	t.Helper()
	values, message, err := renderWithCatalog(t, setupSettings("PREVIEW", "NONE", false), "b1-"+strings.Repeat("a", 64), catalog)
	if err != nil {
		t.Fatalf("render: %s (%v)", message, err)
	}
	decoded, err := configuration.DecodeCatalog([]byte(values["catalog"]))
	if err != nil {
		t.Fatal("runtime catalog schema changed", err)
	}
	c, err := configuration.Decode([]byte(values["configuration"]), decoded)
	if err != nil {
		t.Fatal(err)
	}
	return values, decoded, c
}

func TestReleaseCatalogRoutesCurrentAndHistoricalServiceNames(t *testing.T) {
	catalog := catalogFixture()
	catalog["mysqlServices"] = releaseMetadata(t)["resourceServices"].(map[string]any)["mysqldbsystem"]
	values, decoded, _ := renderReleaseCatalog(t, catalog)
	policy, err := routing.New([]byte(values["configuration"]), decoded)
	if err != nil {
		t.Fatal(err)
	}
	id, compartment, platform := "ocid1.mysqldbsystem.oc1.phx.example", "ocid1.compartment.oc1..example", "IAAS"
	lookups := map[string]routing.Lookup{id: {State: routing.Found, Metadata: routing.Metadata{
		ID: &id, CompartmentID: &compartment, DefinedTags: map[string]map[string]any{"Operations": {"Owner": "payments"}},
	}}}
	for _, service := range []string{currentMySQLService, historicalMySQLService, "Oracle HeatWave MySQL", "Oracle HeatWave MySQL on AWS", currentMySQLService + " unknown"} {
		t.Run(service, func(t *testing.T) {
			n, err := notices.Normalize(notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE",
				Services: []string{service}, Platform: &platform, Resources: []notices.Resource{{ResourceID: id}}})
			if err != nil {
				t.Fatal(err)
			}
			assessment, targets := policy.Targets(n, false)
			plan := policy.Resolve(n, false, lookups)
			if service == "Oracle HeatWave MySQL on AWS" || strings.HasSuffix(service, " unknown") {
				if assessment.State != routing.ReviewRequired || assessment.Reason != routing.ReasonService || len(targets) != 0 || len(plan.Groups) != 0 {
					t.Fatal("unknown service escaped review")
				}
				return
			}
			want := []routing.Target{{ResourceID: id, Region: "us-phoenix-1"}}
			if assessment.State != routing.SupportedShape || !reflect.DeepEqual(targets, want) || !reflect.DeepEqual(plan.Groups, []routing.Group{{DestinationID: "payments", Resources: want}}) {
				t.Fatalf("supported service lost tag routing: assessment=%+v targets=%v groups=%v", assessment, targets, plan.Groups)
			}
		})
	}
}

func TestReleaseServiceExpansionPreservesEnvironmentButChangesRouting(t *testing.T) {
	_, _, old := renderReleaseCatalog(t, catalogFixture())
	expanded := catalogFixture()
	expanded["mysqlServices"] = releaseMetadata(t)["resourceServices"].(map[string]any)["mysqldbsystem"]
	_, _, current := renderReleaseCatalog(t, expanded)
	location := configuration.LedgerLocation{Namespace: "example", Bucket: "ledger"}
	oldEnvironment, err := old.EnvironmentID(location)
	if err != nil {
		t.Fatal(err)
	}
	currentEnvironment, err := current.EnvironmentID(location)
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration, err := old.RoutingGeneration()
	if err != nil {
		t.Fatal(err)
	}
	currentGeneration, err := current.RoutingGeneration()
	if err != nil {
		t.Fatal(err)
	}
	if oldEnvironment != currentEnvironment || old.InstallationID != current.InstallationID || oldGeneration == currentGeneration {
		t.Fatal("service expansion must change routing identity without replacing environment or installation")
	}
}
