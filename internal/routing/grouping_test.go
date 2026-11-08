package routing_test

import (
	"reflect"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestGroupOwnersAndRetainUnresolvedSubset(t *testing.T) {
	ids := []string{
		"ocid1.mysqldbsystem.oc1.iad.a", "ocid1.mysqldbsystem.oc1.iad.b",
		"ocid1.mysqldbsystem.oc1.iad.c", "ocid1.mysqldbsystem.oc1.iad.d",
		"ocid1.mysqldbsystem.oc1.iad.e", "ocid1.mysqldbsystem.oc1.iad.replacement",
	}
	n := notice(t, record(ids[3], ids[1], ids[5], ids[0], ids[4], ids[2], ids[0]))
	lookups := map[string]routing.Lookup{
		ids[0]: metadata(ids[0], "payments"), ids[1]: metadata(ids[1], "video"),
		ids[2]: metadata(ids[2], "billing"), ids[3]: {State: routing.NotFound},
		ids[4]: metadata(ids[4], "unmapped"), ids[5]: metadata(ids[5], "payments"),
		db: metadata(db, "video"), // Extra metadata must never become a recipient/resource.
	}
	p := policy(t, "defined")
	got := p.Resolve(n, false, lookups)
	want := []routing.Group{
		{DestinationID: "payments", Resources: []routing.Target{{ResourceID: ids[0], Region: "us-ashburn-1"}, {ResourceID: ids[2], Region: "us-ashburn-1"}, {ResourceID: ids[5], Region: "us-ashburn-1"}}},
		{DestinationID: "video", Resources: []routing.Target{{ResourceID: ids[1], Region: "us-ashburn-1"}}},
	}
	if !reflect.DeepEqual(got.Groups, want) || len(got.Decisions) != len(ids) {
		t.Fatalf("group membership/order: %+v", got)
	}
	for i, d := range got.Decisions {
		if d.Index != i || d.ResourceID != ids[i] {
			t.Fatalf("unstable resource reference: %+v", d)
		}
		if i == 3 || i == 4 {
			if d.Disposition != routing.Unresolved || d.DestinationID != "" {
				t.Fatalf("unresolved resource assigned: %+v", d)
			}
		} else if d.Disposition != routing.Ready || d.DestinationID == "" {
			t.Fatalf("ready resource lost: %+v", d)
		}
	}
	for range 10 {
		if !reflect.DeepEqual(p.Resolve(n, false, lookups), got) {
			t.Fatal("iteration order changed result")
		}
	}
}

func TestCurrentOwnershipChangesOnlyNewProvisionalResults(t *testing.T) {
	p, n := policy(t, "defined"), notice(t, record(db))
	lookup := metadata(db, "payments")
	lookups := map[string]routing.Lookup{db: lookup}
	before := p.Resolve(n, false, lookups)
	lookup.Metadata.DefinedTags["Operations"]["Owner"] = "video"
	after := p.Resolve(n, false, lookups)
	if len(before.Groups) != 1 || len(after.Groups) != 1 || before.Groups[0].DestinationID != "payments" || after.Groups[0].DestinationID != "video" || before.Revision != after.Revision || before.RoutingGeneration != after.RoutingGeneration {
		t.Fatalf("current ownership or identity: %+v %+v", before, after)
	}
}
