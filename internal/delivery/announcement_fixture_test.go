package delivery

import (
	"os"
	"slices"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

const announcementDB = "ocid1.mysqldbsystem.oc1.iad.example"
const announcementInstance = "ocid1.instance.oc1.iad.example"

// Exercise payload packing independently with a legacy policy. Public Build's
// broader category/type admission is covered by the selected-policy tests.
func announcementFixture(t *testing.T, category, reference string, ids ...string) (*Builder, notices.Record, routing.Plan, []routing.Group) {
	t.Helper()
	data, err := os.ReadFile("testdata/config.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(data, configuration.Catalog{Regions: map[string]string{"us-ashburn-1": "iad"}, MySQLServices: []string{"Oracle HeatWave MySQL"}}, configuration.LedgerLocation{Namespace: "example_namespace", Bucket: "Router.Ledger"})
	if err != nil {
		t.Fatal(err)
	}
	r := notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: category, Reference: &reference}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	var targets []routing.Target
	for _, id := range ids {
		r.Resources = append(r.Resources, notices.Resource{ResourceID: id})
		targets = append(targets, routing.Target{ResourceID: id, Region: "us-ashburn-1"})
	}
	n, err := notices.Normalize(r)
	if err != nil {
		t.Fatal(err)
	}
	return b, n.Snapshot(), routing.Plan{Revision: n.Key(), RoutingGeneration: b.policy.Resolve(n, false, nil).RoutingGeneration}, []routing.Group{{DestinationID: "payments", Resources: targets}}
}

func announcementPreparation(t *testing.T, category, reference string, ids ...string) Preparation {
	t.Helper()
	b, r, plan, groups := announcementFixture(t, category, reference, ids...)
	result := b.pack(r, plan, groups)
	if len(result.Drafts) != 1 || len(result.Issues) != 0 {
		t.Fatal("expected one draft", result)
	}
	return Preparation{Draft: result.Drafts[0], SecretVersion: 7}
}

func announcementRecord(t *testing.T, category, reference string, ids ...string) Record {
	t.Helper()
	r, err := NewRecord(announcementPreparation(t, category, reference, ids...))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
