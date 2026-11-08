package routing_test

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/routing"
)

func TestMetadataFailuresRemainVisible(t *testing.T) {
	p, n := policy(t, "defined"), notice(t, record(db))
	for _, tc := range []struct {
		name   string
		change func(*routing.Lookup)
		reason routing.Reason
	}{
		{"denied", func(l *routing.Lookup) { l.State = routing.Denied }, routing.ReasonLookupDenied},
		{"not found is not deletion", func(l *routing.Lookup) { l.State = routing.NotFound }, routing.ReasonLookupNotFound},
		{"unavailable", func(l *routing.Lookup) { l.State = routing.Unavailable }, routing.ReasonLookupUnavailable},
		{"zero state", func(l *routing.Lookup) { l.State = "" }, routing.ReasonLookupUnavailable},
		{"unknown state", func(l *routing.Lookup) { l.State = "private-error" }, routing.ReasonLookupUnavailable},
		{"missing id", func(l *routing.Lookup) { l.Metadata.ID = nil }, routing.ReasonIdentityMismatch},
		{"different id", func(l *routing.Lookup) { l.Metadata.ID = text(db + "other") }, routing.ReasonIdentityMismatch},
		{"equivalent spelling not exact id", func(l *routing.Lookup) { l.Metadata.ID = text("ocid1.mysqldbsystem.oc1.us-ashburn-1.example") }, routing.ReasonIdentityMismatch},
		{"missing compartment", func(l *routing.Lookup) { l.Metadata.CompartmentID = nil }, routing.ReasonCompartment},
		{"empty compartment", func(l *routing.Lookup) { l.Metadata.CompartmentID = text("") }, routing.ReasonCompartment},
		{"regional compartment", func(l *routing.Lookup) { l.Metadata.CompartmentID = text("ocid1.compartment.oc1.iad.other") }, routing.ReasonCompartment},
		{"wrong realm", func(l *routing.Lookup) { l.Metadata.CompartmentID = text("ocid1.compartment.oc2..other") }, routing.ReasonCompartment},
		{"malformed compartment", func(l *routing.Lookup) { l.Metadata.CompartmentID = text("ocid1.compartment.oc1..private/value") }, routing.ReasonCompartment},
		{"oversize compartment", func(l *routing.Lookup) { l.Metadata.CompartmentID = text(strings.Repeat("x", (1<<20)+1)) }, routing.ReasonCompartment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := metadata(db, "payments")
			tc.change(&lookup)
			got := p.Resolve(n, false, map[string]routing.Lookup{db: lookup})
			if len(got.Decisions) != 1 {
				t.Fatalf("missing outcome: %+v", got)
			}
			d := got.Decisions[0]
			if len(got.Groups) != 0 || d.Disposition != routing.Unresolved || d.Reason != tc.reason || d.DestinationID != "" {
				t.Fatalf("metadata failure escaped review: %+v", got)
			}
		})
	}
	got := p.Resolve(n, false, nil)
	if len(got.Decisions) != 1 || got.Decisions[0].Reason != routing.ReasonLookupMissing || len(got.Groups) != 0 {
		t.Fatalf("missing lookup: %+v", got)
	}
}

func TestProvenDisallowedCompartmentIsOutOfScope(t *testing.T) {
	lookup := metadata(db, "payments")
	lookup.Metadata.CompartmentID = text("ocid1.compartment.oc1..other")
	got := policy(t, "defined").Resolve(notice(t, record(db)), false, map[string]routing.Lookup{db: lookup})
	if len(got.Decisions) != 1 || got.Decisions[0].Disposition != routing.OutOfScope || got.Decisions[0].Reason != routing.ReasonCompartmentScope || len(got.Groups) != 0 {
		t.Fatalf("compartment scope ignored: %+v", got)
	}
}
