package noticeledger

import (
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
)

func TestSavedReviewDerivesAssessmentAndBatchReview(t *testing.T) {
	raw := source()
	raw.AnnouncementType = "UNSUPPORTED"
	r := observed(t, raw)
	view, err := r.Review(PreparationIssues{}, strings.Repeat("a", 64))
	if err != nil || len(view.Issues) != 1 || view.Issues[0].Kind != "ASSESSMENT" {
		t.Fatal("unsupported source disappeared", view, err)
	}
	if _, err = observer(t, nil).PreparationFailure(r, normalized(t, raw), false, nil, []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("unsupported source prepared")
	}
	r = observed(t, source(db), db)
	r, err = r.Prepare(clock(), preparations(t, source(db), db)...)
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Hold(delivery.ReasonOperator, clock())
	if err != nil {
		t.Fatal(err)
	}
	view, err = r.Review(PreparationIssues{}, strings.Repeat("a", 64))
	if err != nil || len(view.Issues) != 1 || view.Issues[0].Kind != "BATCH" || view.Issues[0].Code != "operator" || view.Issues[0].BatchID != r.Snapshot().Batches[0].Message.BatchID {
		t.Fatal("batch review missing or duplicated", view, err)
	}
}

func TestSavedReviewKeepsUnavailableAndUnknownSourcesVisible(t *testing.T) {
	r := observed(t, source(db), db)
	unavailable, err := r.RecordSourceUnavailable(404, clock())
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := legacyRecord(t, r).MigrateSourceOutcome(clock())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []Record{unavailable, unknown} {
		view, err := record.Review(PreparationIssues{}, strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, issue := range view.Issues {
			if issue.Kind == "SOURCE" {
				found = true
			}
		}
		if !found {
			t.Fatal("saved source gap hidden", view)
		}
	}
}

func TestSavedReviewRejectsForeignSidecarsAndInvalidInputs(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_invalid", clock(), 0))
	if err != nil {
		t.Fatal(err)
	}
	other := source(db, otherDB)
	other.NoticeID = "ocid1.announcement.oc1..other"
	foreign := observed(t, other, db, otherDB)
	if _, err = foreign.Review(p, strings.Repeat("a", 64)); err == nil {
		t.Fatal("foreign sidecar accepted")
	}
	if _, err = (Record{}).Review(p, strings.Repeat("a", 64)); err == nil {
		t.Fatal("empty notice accepted")
	}
	if _, err = r.Review(p, "invalid"); err == nil {
		t.Fatal("invalid current digest accepted")
	}
}
