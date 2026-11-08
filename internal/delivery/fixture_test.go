package delivery_test

import (
	"encoding/json"
	"os"
	"testing"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

const db = "ocid1.mysqldbsystem.oc1.iad.example"
const compartment = "ocid1.compartment.oc1..example"

func text(value string) *string { return &value }

func catalog() configuration.Catalog {
	return configuration.Catalog{
		Regions:       map[string]string{"us-ashburn-1": "iad", "us-phoenix-1": "phx"},
		MySQLServices: []string{"Oracle HeatWave MySQL", "MySQL Database"},
	}
}

func ledger() configuration.LedgerLocation {
	return configuration.LedgerLocation{Namespace: "example_namespace", Bucket: "Router.Ledger"}
}

func settings(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/config.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(value)
	}
	data, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func builder(t *testing.T, change func(map[string]any)) *delivery.Builder {
	t.Helper()
	b, err := delivery.New(settings(t, change), catalog(), ledger())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func record(ids ...string) notices.Record {
	r := notices.Record{NoticeID: "ocid1.announcement.oc1..example", AnnouncementType: "SCHEDULED_MAINTENANCE",
		Services: []string{"Oracle HeatWave MySQL"}, Platform: text("IAAS")}
	for _, id := range ids {
		r.Resources = append(r.Resources, notices.Resource{ResourceID: id})
	}
	return r
}

func notice(t *testing.T, r notices.Record) notices.Notice {
	t.Helper()
	n, err := notices.Normalize(r)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func metadata(id, owner string) routing.Lookup {
	return routing.Lookup{State: routing.Found, Metadata: routing.Metadata{
		ID: text(id), CompartmentID: text(compartment),
		DefinedTags: map[string]map[string]any{"Operations": {"Owner": owner}},
	}}
}

func build(t *testing.T, b *delivery.Builder, n notices.Notice, lookups map[string]routing.Lookup, ids ...string) delivery.Result {
	t.Helper()
	got, err := b.Build(n, false, lookups, ids)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func single(t *testing.T, got delivery.Result) delivery.Snapshot {
	t.Helper()
	if len(got.Drafts) != 1 || len(got.Issues) != 0 {
		t.Fatalf("expected one draft, got %+v", got)
	}
	return got.Drafts[0].Snapshot()
}

func envelope(t *testing.T, s delivery.Snapshot) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(s.PayloadWithoutRoutingKey, &got); err != nil {
		t.Fatal(err)
	}
	return got
}
