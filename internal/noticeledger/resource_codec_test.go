package noticeledger

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestReloadPreservesSupportedAndUnresolvedResourceForms(t *testing.T) {
	for _, ids := range [][]string{{}, {db}, {"invalid private ID"}, {"ocid1.mysqldbsystem.oc1.unknown.example"}, {db, "invalid private ID"}} {
		r := observed(t, source(ids...))
		reload(t, r)
		held, e := r.Hold(delivery.ReasonSourceUnavailable, clock())
		if e != nil {
			t.Fatal(e)
		}
		reload(t, held)
	}
	r := source(db)
	r.AnnouncementType = "SERVICE_DISRUPTION"
	reload(t, observed(t, r, db))
	r = source(db)
	r.TimeUpdated = text("0000-01-01T00:00:00Z")
	r.ChainID = text("opaque<>&🐬")
	reload(t, observed(t, r, db))
	lookup := lookups(db)
	x := lookup[db]
	x.Metadata.CompartmentID = text("ocid1.compartment.oc1..outside")
	lookup[db] = x
	outside, e := observer(t, nil).Observe(Record{}, normalized(t, source(db)), false, lookup, clock())
	if e != nil {
		t.Fatal(e)
	}
	if outside.Snapshot().Revisions[0].Resources[0].Disposition != "OUT_OF_SCOPE" {
		t.Fatal("scope decision lost")
	}
	reload(t, outside)
	ready := observed(t, source(db), db)
	reload(t, ready)
	empty, e := ready.Prepare(clock())
	if e != nil || len(empty.Snapshot().Batches) != 0 || !empty.Follows(ready) {
		t.Fatal("empty preparation changed membership")
	}
	reload(t, empty)
}

func TestReloadRejectsForgedResourceAndAssessmentClaims(t *testing.T) {
	r := observed(t, source(db))
	env, id := r.Identity()
	changes := map[string]func(*wireRecord){
		"unknown_assessment": func(w *wireRecord) { w.Revisions[0].Assessment = "unknown" },
		"unknown_review_reason": func(w *wireRecord) {
			w.Revisions[0].Assessment = routing.ReviewRequired
			w.Revisions[0].AssessmentReason = "private"
		},
		"invalid_reason":         func(w *wireRecord) { w.Revisions[0].Resources[0].Reason = "private" },
		"invalid_state":          func(w *wireRecord) { w.Revisions[0].Resources[0].Disposition = "unknown" },
		"unresolved_destination": func(w *wireRecord) { w.Revisions[0].Resources[0].DestinationID = "payments" },
		"invalid_destination":    func(w *wireRecord) { w.Revisions[0].Resources[0].DestinationID = "Private Value" },
		"invalid_region":         func(w *wireRecord) { w.Revisions[0].Resources[0].Region = "invalid region" },
		"blank_id_with_region":   func(w *wireRecord) { w.Revisions[0].Resources[0].ResourceID = "" },
		"invalid_batch":          func(w *wireRecord) { w.Revisions[0].Resources[0].BatchID = "not-a-batch" },
		"forged_review":          func(w *wireRecord) { w.Revisions[0].Resources[0].Disposition = "REVIEW_REQUIRED" },
		"duplicate_resource": func(w *wireRecord) {
			row := w.Revisions[0].Resources[0]
			row.Index = 1
			w.Revisions[0].Resources = append(w.Revisions[0].Resources, row)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			n, e := Decode(changedWire(t, r, change), env, id)
			rejected(t, n, e)
			if strings.Contains(e.Error(), "Private Value") || strings.Contains(e.Error(), "private") {
				t.Fatal("untrusted diagnostic escaped")
			}
		})
	}
	if env, id := (Record{}).Identity(); env != "" || id != "" {
		t.Fatal("zero record has identity")
	}
}
