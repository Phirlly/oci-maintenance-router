package routing_test

import (
	"testing"

	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestNoticeAssessmentGatesBothOperations(t *testing.T) {
	p := policy(t, "defined")
	for _, tc := range []struct {
		name   string
		change func(*notices.Record)
		known  bool
		state  routing.AssessmentState
		reason routing.Reason
	}{
		{"supported", func(r *notices.Record) {}, false, routing.SupportedShape, ""},
		{"unrelated", func(r *notices.Record) { r.Services = []string{"Compute"}; r.Resources = nil }, false, routing.Unrelated, routing.ReasonUnrelated},
		{"known label changed", func(r *notices.Record) { r.Services = []string{"Compute"}; r.Resources = nil }, true, routing.ReviewRequired, routing.ReasonService},
		{"known type changed", func(r *notices.Record) { r.AnnouncementType = "FUTURE_TYPE" }, true, routing.ReviewRequired, routing.ReasonType},
		{"mysql id fallback", func(r *notices.Record) { r.Services = nil }, false, routing.ReviewRequired, routing.ReasonService},
		{"malformed mysql fallback", func(r *notices.Record) { r.Services = nil; r.Resources[0].ResourceID = "ocid1.mysqldbsystem.unsafe" }, false, routing.ReviewRequired, routing.ReasonService},
		{"nonconfigured catalog label", func(r *notices.Record) { r.Services = []string{"MySQL Database"} }, false, routing.ReviewRequired, routing.ReasonService},
		{"no service substring", func(r *notices.Record) { r.Services = []string{"Prefix Oracle HeatWave MySQL"} }, false, routing.ReviewRequired, routing.ReasonService},
		{"type case", func(r *notices.Record) { r.AnnouncementType = "ScheduledMaintenance" }, false, routing.ReviewRequired, routing.ReasonType},
		{"incident", func(r *notices.Record) { r.AnnouncementType = "INCIDENT" }, false, routing.ReviewRequired, routing.ReasonType},
		{"future update", func(r *notices.Record) { r.AnnouncementType = "MAINTENANCE_RESCHEDULED" }, false, routing.ReviewRequired, routing.ReasonType},
		{"missing platform", func(r *notices.Record) { r.Platform = nil }, false, routing.ReviewRequired, routing.ReasonPlatform},
		{"other platform", func(r *notices.Record) { r.Platform = text("SAAS") }, false, routing.ReviewRequired, routing.ReasonPlatform},
		{"missing resources", func(r *notices.Record) { r.Resources = nil }, false, routing.ReviewRequired, routing.ReasonNoResources},
		{"inactive without times", func(r *notices.Record) { r.LifecycleState = text("INACTIVE") }, false, routing.SupportedShape, ""},
		{"known is not send permission", func(r *notices.Record) {}, true, routing.SupportedShape, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := record(db)
			tc.change(&r)
			n := notice(t, r)
			want := routing.Assessment{State: tc.state, Reason: tc.reason}
			a, targets := p.Targets(n, tc.known)
			got := p.Resolve(n, tc.known, map[string]routing.Lookup{db: metadata(db, "payments")})
			if a != want || got.Assessment != want {
				t.Fatalf("assessment = %+v / %+v, want %+v", a, got.Assessment, want)
			}
			if tc.state == routing.SupportedShape {
				if len(targets) != 1 || len(got.Groups) != 1 {
					t.Fatal("supported shape lost provisional work")
				}
				return
			}
			if len(targets) != 0 || len(got.Groups) != 0 {
				t.Fatal("unsupported shape produced work")
			}
			if tc.state == routing.Unrelated {
				if len(got.Decisions) != 0 {
					t.Fatal("unrelated notice produced resource work")
				}
				return
			}
			if len(got.Decisions) != len(r.Resources) {
				t.Fatal("candidate resources disappeared")
			}
			for _, decision := range got.Decisions {
				if decision.Disposition != routing.HeldForReview || decision.Reason != tc.reason || decision.DestinationID != "" {
					t.Fatalf("unsupported candidate escaped review: %+v", decision)
				}
			}
		})
	}
}
