package routing_test

import (
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestSelectedResourcesRouteAllAnnouncementCategories(t *testing.T) {
	for _, category := range []string{"SCHEDULED_MAINTENANCE", "ACTION_REQUIRED", "PRODUCTION_EVENT_NOTIFICATION", "INFORMATION"} {
		t.Run(category, func(t *testing.T) {
			p := selectedPolicy(t, "mysqldbsystem", "instance")
			r := record(db, instance)
			r.AnnouncementType, r.Services = category, []string{"Oracle HeatWave MySQL", computeService}
			n := notice(t, r)
			a, targets := p.Targets(n, false)
			got := p.Resolve(n, false, map[string]routing.Lookup{db: metadata(db, "payments"), instance: metadata(instance, "video")})
			if a.State != routing.SupportedShape || len(targets) != 2 || len(got.Groups) != 2 {
				t.Fatalf("selected resources lost: %+v %+v", targets, got)
			}
			if got.Groups[0].DestinationID != "payments" || got.Groups[0].Resources[0].ResourceID != db || got.Groups[1].DestinationID != "video" || got.Groups[1].Resources[0].ResourceID != instance {
				t.Fatal("resource leaked to another team", got.Groups)
			}
		})
	}
}

func TestSelectedResourceRequiresItsOwnService(t *testing.T) {
	for _, services := range [][]string{nil, {computeService}, {"Oracle HeatWave MySQL"}, {"unselected service"}} {
		p := selectedPolicy(t, "mysqldbsystem", "instance")
		r := record(db, instance)
		r.Services = services
		n := notice(t, r)
		a, targets := p.Targets(n, false)
		got := p.Resolve(n, false, map[string]routing.Lookup{db: metadata(db, "payments"), instance: metadata(instance, "video")})
		if a.State != routing.SupportedShape || len(got.Decisions) != 2 {
			t.Fatal("selected prefix did not retain per-resource review", got)
		}
		wantTargets := 0
		for _, d := range got.Decisions {
			allowed := len(services) == 1 && ((d.ResourceID == db && services[0] == "Oracle HeatWave MySQL") || (d.ResourceID == instance && services[0] == computeService))
			if allowed {
				wantTargets++
				if d.Disposition != routing.Ready {
					t.Fatal("matching service not routed", d)
				}
			} else if d.Disposition != routing.Unresolved || d.Reason != routing.ReasonService || d.DestinationID != "" {
				t.Fatal("another service authorized resource", d)
			}
		}
		if len(targets) != wantTargets || len(got.Groups) != wantTargets {
			t.Fatal("unexpected lookup or recipient", targets, got.Groups)
		}
	}
}
