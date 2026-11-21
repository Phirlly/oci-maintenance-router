package routing_test

import (
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestSelectedResourceScopeBeforeLookup(t *testing.T) {
	p := selectedPolicy(t, "instance")
	for _, tc := range []struct {
		id, region  string
		disposition routing.Disposition
		reason      routing.Reason
		safe        bool
	}{
		{instance, "us-ashburn-1", routing.Ready, "", true},
		{"ocid1.instance.oc1.us-ashburn-1..replacement", "us-ashburn-1", routing.Ready, "", true},
		{db, "us-ashburn-1", routing.OutOfScope, "resource_type_out_of_scope", true},
		{"ocid1.mysqldbsystem.oc1.phx.example", "us-phoenix-1", routing.OutOfScope, routing.ReasonRegionScope, true},
		{"ocid1.instance.oc1.phx.example", "us-phoenix-1", routing.OutOfScope, routing.ReasonRegionScope, true},
		{"ocid1.mysqldbsystem.oc1.unknown.example", "", routing.Unresolved, routing.ReasonUnknownRegion, true},
		{"ocid1.instance.oc1.unknown.example", "", routing.Unresolved, routing.ReasonUnknownRegion, true},
		{"ocid1.instance.oc2.iad.example", "", routing.Unresolved, routing.ReasonResourceID, false},
		{"ocid1.instance.oc1.iad.a.b.c", "", routing.Unresolved, routing.ReasonResourceID, false},
		{"ocid1.dbsystem.oc1.iad.example", "", routing.Unresolved, routing.ReasonResourceID, false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r := record(tc.id)
			r.Services = []string{computeService}
			n := notice(t, r)
			_, targets := p.Targets(n, false)
			got := p.Resolve(n, false, map[string]routing.Lookup{tc.id: metadata(tc.id, "payments")})
			d := got.Decisions[0]
			if d.Disposition != tc.disposition || d.Reason != tc.reason || d.Region != tc.region || (d.ResourceID != "") != tc.safe {
				t.Fatal("scope decision", d)
			}
			if tc.disposition == routing.Ready {
				if len(targets) != 1 || len(got.Groups) != 1 {
					t.Fatal("selected target lost", got)
				}
			} else if len(targets) != 0 || len(got.Groups) != 0 {
				t.Fatal("out-of-scope work", targets, got.Groups)
			}
		})
	}
}

func TestSelectionKeepsUnrelatedAndGeneralNoticeBoundaries(t *testing.T) {
	p := selectedPolicy(t, "instance")
	r := record(db)
	if a, targets := p.Targets(notice(t, r), false); a.State != routing.Unrelated || len(targets) != 0 {
		t.Fatal("unselected type seeded work", a)
	}
	r = record()
	r.Services = []string{computeService}
	if a, targets := p.Targets(notice(t, r), false); a.Reason != routing.ReasonNoResources || len(targets) != 0 {
		t.Fatal("general notice created work", a)
	}
	r = record(instance)
	r.Services, r.Platform = []string{computeService}, nil
	if a, targets := p.Targets(notice(t, r), false); a.Reason != routing.ReasonPlatform || len(targets) != 0 {
		t.Fatal("unsupported platform created work", a)
	}
}
