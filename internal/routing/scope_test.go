package routing_test

import (
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestResourceScopeBeforeLookup(t *testing.T) {
	p := policy(t, "defined")
	for _, tc := range []struct {
		id          string
		disposition routing.Disposition
		reason      routing.Reason
		region      string
		safe        bool
	}{
		{db, routing.Ready, "", "us-ashburn-1", true},
		{"ocid1.mysqldbsystem.oc1.us-ashburn-1..replacement", routing.Ready, "", "us-ashburn-1", true},
		{"ocid1.mysqldbsystem.oc1.iad.future.unique_-", routing.Ready, "", "us-ashburn-1", true},
		{"ocid1.mysqldbsystem.oc1.phx.other", routing.OutOfScope, routing.ReasonRegionScope, "us-phoenix-1", true},
		{"ocid1.mysqldbsystem.oc1.us-phoenix-1.other", routing.OutOfScope, routing.ReasonRegionScope, "us-phoenix-1", true},
		{"ocid1.mysqldbsystem.oc1.unknown.other", routing.Unresolved, routing.ReasonUnknownRegion, "", true},
		{"ocid1.mysqldbsystem.oc2.iad.other", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.instance.oc1.iad.other", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.dbsystem.oc1.iad.other", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.mysqldbsystem.oc1..other", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.mysqldbsystem.oc1.iad.", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.mysqldbsystem.oc1.iad.a.b.c", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"ocid1.mysqldbsystem.oc1.iad.path/other", routing.Unresolved, routing.ReasonResourceID, "", false},
		{"private\nresource", routing.Unresolved, routing.ReasonResourceID, "", false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			n := notice(t, record(tc.id))
			_, targets := p.Targets(n, false)
			got := p.Resolve(n, false, map[string]routing.Lookup{tc.id: metadata(tc.id, "payments")})
			if len(got.Decisions) != 1 {
				t.Fatalf("resource disappeared: %+v", got)
			}
			d := got.Decisions[0]
			if d.Index != 0 || d.Disposition != tc.disposition || d.Reason != tc.reason || d.Region != tc.region || (d.ResourceID != "") != tc.safe {
				t.Fatalf("scope decision: %+v", d)
			}
			if tc.safe && d.ResourceID != tc.id {
				t.Fatal("identifier rewritten")
			}
			if tc.disposition == routing.Ready {
				if len(targets) != 1 || targets[0].ResourceID != tc.id || targets[0].Region != tc.region || len(got.Groups) != 1 {
					t.Fatal("allowed target lost")
				}
			} else if len(targets) != 0 || len(got.Groups) != 0 || d.DestinationID != "" {
				t.Fatal("disallowed resource produced work")
			}
		})
	}
}

func TestDisplayRegionCannotSelectEndpoint(t *testing.T) {
	r := record(db)
	r.Resources[0].RegionDisplay = text("us-phoenix-1")
	_, targets := policy(t, "defined").Targets(notice(t, r), false)
	if len(targets) != 1 || targets[0].Region != "us-ashburn-1" {
		t.Fatal("display label changed endpoint")
	}
}
