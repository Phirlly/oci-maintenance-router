package noticeledger

import (
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestPreparationReviewRequiresRecheckAfterOwnershipAndSourceChanges(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_invalid", clock(), 0))
	if err != nil {
		t.Fatal(err)
	}
	config := preparationConfigDigest()
	changed := lookups(db, otherDB)
	lookup := changed[db]
	lookup.Metadata.FreeformTags["owner"] = "video"
	changed[db] = lookup
	owner, err := observer(t, nil).Observe(r, normalized(t, source(db, otherDB)), false, changed, clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := observer(t, nil).Observe(r, normalized(t, source(db, otherDB)), false, nil, clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	raw := source(db, otherDB)
	raw.Reference = text("new revision")
	sourceChanged, err := observer(t, nil).Observe(r, normalized(t, raw), false, lookups(db, otherDB), clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []Record{owner, unresolved, sourceChanged, legacyRecord(t, r)} {
		view, err := state.Review(p, config)
		if err != nil || view.Preparation[0].Status != "RECHECK_REQUIRED" {
			t.Fatal("changed context resolved historical failure", view, err)
		}
	}
}

func TestSavedReviewKeepsMalformedRowsDistinctAndSuppressesAcceptedBatches(t *testing.T) {
	r := observed(t, source("bad-a", "bad-b"))
	view, err := r.Review(PreparationIssues{}, strings.Repeat("a", 64))
	if err != nil || len(view.Issues) != 2 || view.Issues[0].ResourceIndex == view.Issues[1].ResourceIndex || view.Issues[0].Code != string(routing.ReasonResourceID) {
		t.Fatal("malformed rows conflated", view, err)
	}
	r = observed(t, source(db), db)
	r = prepare(t, r, preparations(t, source(db), db)...)
	sending, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, err := sending.Finish(ref, 202, clock(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err = accepted.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	view, err = accepted.Review(PreparationIssues{}, strings.Repeat("a", 64))
	if err != nil || len(view.Issues) != 0 {
		t.Fatal("accepted work still actionable", view, err)
	}
}
