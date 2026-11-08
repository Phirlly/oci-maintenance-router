package delivery_test

import (
	"reflect"
	"strings"
	"testing"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/notices"
	"oci-maintenance-router/internal/routing"
)

func TestAssignmentsSurvivePartialSuccessAndLaterResources(t *testing.T) {
	ids := []string{db, db + "other", db + "later"}
	n := notice(t, record(ids...))
	m := map[string]routing.Lookup{db: metadata(db, "payments"), ids[1]: metadata(ids[1], "video"), ids[2]: metadata(ids[2], "payments")}
	b := builder(t, nil)
	initial := build(t, b, n, m, ids[:2]...)
	r, err := delivery.NewRecord(delivery.Preparation{Draft: initial.Drafts[0], SecretVersion: 7}, delivery.Preparation{Draft: initial.Drafts[1], SecretVersion: 8})
	if err != nil {
		t.Fatal(err)
	}
	sent, ref, err := attempt(r, instant())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := sent.Finish(ref, 202, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := accepted.Unassigned(n.Key(), ids)
	if err != nil || !reflect.DeepEqual(remaining, ids[2:]) {
		t.Fatalf("assignment lost: %v %v", remaining, err)
	}
	late := build(t, b, n, m, remaining...)
	after, err := accepted.Prepare(delivery.Preparation{Draft: late.Drafts[0], SecretVersion: 9})
	if err != nil || len(after.Batches()) != 3 || len(accepted.Batches()) != 2 {
		t.Fatalf("late assignment failed: %v", err)
	}
	saved, _ := after.Batch(ref.BatchID)
	if saved.State != delivery.APIAccepted || len(saved.Message.Resources) != 1 {
		t.Fatal("receipt grew or reset")
	}
	remaining, err = after.Unassigned(n.Key(), ids)
	if err != nil || len(remaining) != 0 {
		t.Fatal("assigned IDs reselected")
	}
	other := n.Key()
	other.Digest = strings.Repeat("a", 64)
	remaining, err = after.Unassigned(other, ids)
	if err != nil || !reflect.DeepEqual(remaining, []string{db, db + "later", db + "other"}) {
		t.Fatal("distinct revision conflated")
	}
}

func TestDistinctBatchesCannotOverlapInAnyState(t *testing.T) {
	n := notice(t, record(db, db+"other"))
	b := builder(t, nil)
	m := map[string]routing.Lookup{db: metadata(db, "payments"), db + "other": metadata(db+"other", "payments")}
	first := build(t, b, n, m, db)
	r, err := delivery.NewRecord(delivery.Preparation{Draft: first.Drafts[0], SecretVersion: 7})
	if err != nil {
		t.Fatal(err)
	}
	sent, ref, err := attempt(r, instant())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := sent.Finish(ref, 202, instant(), zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	held, err := r.Hold(ref.BatchID, delivery.ReasonOperator, instant())
	if err != nil {
		t.Fatal(err)
	}
	overlap := build(t, b, n, m, db, db+"other")
	m[db] = metadata(db, "video")
	reassigned := build(t, b, n, m, db)
	for _, base := range []delivery.Record{r, accepted, held} {
		for _, draft := range []delivery.Draft{overlap.Drafts[0], reassigned.Drafts[0]} {
			got, err := base.Prepare(delivery.Preparation{Draft: draft, SecretVersion: 9})
			rejectedRecord(t, got, err)
		}
	}
}

func TestUnassignedRejectsUntrustedSelection(t *testing.T) {
	r, values := prepared(t, db)
	rev := values[0].Draft.Snapshot().Revision
	for _, ids := range [][]string{{db, db}, {"private"}, make([]string, 257), {strings.Repeat("x", 1<<20+1)}} {
		got, err := r.Unassigned(rev, ids)
		if err == nil || got != nil {
			t.Fatal("bad selection accepted")
		}
	}
	for _, key := range []notices.RevisionKey{{}, {NoticeID: rev.NoticeID, Digest: "bad"}, {NoticeID: "ocid1.announcement.oc1..other", Digest: rev.Digest}} {
		got, err := r.Unassigned(key, nil)
		if err == nil || got != nil {
			t.Fatal("foreign revision accepted")
		}
	}
	if got, err := (delivery.Record{}).Unassigned(rev, nil); err == nil || got != nil {
		t.Fatal("zero record selected work")
	}
}
