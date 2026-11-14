package noticeledger

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/notices"
)

func TestPreparationFailureRequiresExactCurrentRoutingAndRetainedRows(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	n := normalized(t, source(db, otherDB))
	for _, o := range []*Observer{nil, {}} {
		if _, err := o.PreparationFailure(r, n, false, lookups(db, otherDB), []int{0}, "secret_invalid", clock()); err == nil {
			t.Fatal("unbound observer accepted")
		}
	}
	o := observer(t, nil)
	if _, err := o.PreparationFailure(Record{}, n, false, lookups(db, otherDB), []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("empty record accepted")
	}
	if _, err := o.PreparationFailure(r, notices.Notice{}, false, lookups(db, otherDB), []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := o.PreparationFailure(r, n, false, nil, []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("unresolved owner became preparation evidence")
	}
	if _, err := o.PreparationFailure(r, n, false, lookups(db, otherDB), []int{0}, "raw provider error", clock()); err == nil {
		t.Fatal("uncontrolled cause admitted")
	}
	changed := source(db, otherDB)
	changed.Reference = text("changed")
	if _, err := o.PreparationFailure(r, normalized(t, changed), false, lookups(db, otherDB), []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("unretained revision accepted")
	}
	prepared, err := r.Prepare(clock(), preparations(t, source(db, otherDB), db, otherDB)...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.PreparationFailure(prepared, n, false, lookups(db, otherDB), []int{0}, "secret_invalid", clock()); err == nil {
		t.Fatal("assigned work got preparation evidence")
	}
}

func TestPreparationIssuesCannotRewindObservationOrBorrowForeignState(t *testing.T) {
	r := observed(t, source(db, otherDB), db, otherDB)
	p, err := (PreparationIssues{}).Observe(preparationFailure(t, r, "secret_invalid", clock().Add(time.Second), 0))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := p.Encode()
	if got, err := p.Observe(preparationFailure(t, r, "secret_unavailable", clock(), 0)); err == nil || got.Snapshot().SchemaVersion != 0 {
		t.Fatal("observation clock rewound")
	}
	other := source(db, otherDB)
	other.NoticeID = "ocid1.announcement.oc1..other"
	r2 := observed(t, other, db, otherDB)
	f, err := observer(t, nil).PreparationFailure(r2, normalized(t, other), false, lookups(db, otherDB), []int{0}, "secret_invalid", clock().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Observe(f); err == nil {
		t.Fatal("cross-notice observation accepted")
	}
	after, _ := p.Encode()
	if string(before) != string(after) {
		t.Fatal("failed proposal mutated history")
	}
}
