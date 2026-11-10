package noticeledger

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestCapturedApplyPreservesAssignedReceiptsAndFirstObservation(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db, otherDB)
	r = prepare(t, r, preparations(t, src, db)...)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, e := r.Finish(ref, 202, clock(), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	before := accepted.Snapshot()
	oldBytes := encoded(t, accepted)
	src.TimeUpdated = text("2026-02-01T00:00:00Z")
	o := captured(t, normalized(t, src))
	next, e := o.Apply(accepted, clock().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	after := next.Snapshot()
	if !next.Follows(accepted) || next.Follows(r) || !reflect.DeepEqual(before.Batches, after.Batches) || !bytes.Equal(oldBytes, encoded(t, accepted)) {
		t.Fatal("assignment, predecessor or receipt changed")
	}
	if after.Revisions[0].FirstSeenAt != before.Revisions[0].FirstSeenAt || after.Revisions[0].LastSeenAt == before.Revisions[0].LastSeenAt || *after.Revisions[0].Source.TimeUpdated != "2026-02-01T00:00:00.000Z" {
		t.Fatal("observation times/context changed incorrectly")
	}
	if after.Revisions[0].Resources[0].Disposition != "API_ACCEPTED" || after.Revisions[0].Resources[1].Disposition != string(routing.Unresolved) {
		t.Fatal("capture changed receipt or invented current ownership")
	}
	reload(t, next)
}

func TestCapturedDistinctRevisionRetainsStickyHoldAndAmbiguity(t *testing.T) {
	src := source(db)
	r := prepare(t, observed(t, src, db), preparations(t, src, db)...)
	r, ref := begin(t, r, r.Snapshot().Batches[0].Message.BatchID, clock())
	r, e := r.Finish(ref, 0, clock(), clock())
	if e != nil {
		t.Fatal(e)
	}
	original := captured(t, normalized(t, src))
	src.Reference = text("changed")
	changed := captured(t, normalized(t, src))
	held := applyCaptured(t, changed, r)
	if len(held.Snapshot().Revisions) != 2 || held.Snapshot().HoldReason != delivery.ReasonSourceChanged || !held.Snapshot().Batches[0].Ambiguous || held.Snapshot().Batches[0].State != delivery.ReviewRequired {
		t.Fatal("change lost history or remained retryable")
	}
	repeated := applyCaptured(t, original, held)
	if repeated.Snapshot().HoldReason != delivery.ReasonSourceChanged || len(repeated.Snapshot().Revisions) != 2 {
		t.Fatal("old capture cleared history")
	}
	reload(t, repeated)
}

func TestCapturedApplyRejectsForeignIdentityAndInvalidMergeTime(t *testing.T) {
	base := observed(t, source(db))
	prior := encoded(t, base)
	foreignSource := source(db)
	foreignSource.NoticeID = "ocid1.announcement.oc1..foreign"
	foreign := captured(t, normalized(t, foreignSource))
	next, e := foreign.Apply(base, clock())
	rejected(t, next, e)
	o := observer(t, func(v map[string]any) { v["tenancyId"] = "ocid1.tenancy.oc1..foreign" })
	foreign, e = o.Capture(normalized(t, source(db)))
	if e != nil {
		t.Fatal(e)
	}
	next, e = foreign.Apply(base, clock())
	rejected(t, next, e)
	capture := captured(t, normalized(t, source(db)))
	for _, at := range []time.Time{{}, clock().Add(-time.Second), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		next, e = capture.Apply(base, at)
		rejected(t, next, e)
	}
	if !bytes.Equal(prior, encoded(t, base)) {
		t.Fatal("failed merge changed state")
	}
}
