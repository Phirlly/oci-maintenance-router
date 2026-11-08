package noticeledger

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func TestChangedRevisionPreservesReceiptAndHoldsInflightRecovery(t *testing.T) {
	src := source(db, otherDB)
	r := observed(t, src, db, otherDB)
	one, two := preparations(t, src, db), preparations(t, src, otherDB)
	r = prepare(t, r, append(one, two...)...)
	r, ref := begin(t, r, one[0].Draft.Snapshot().BatchID, clock())
	r, e := r.Finish(ref, 202, clock(), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	r, inflight := begin(t, r, two[0].Draft.Snapshot().BatchID, clock())
	changed := src
	changed.Reference = text("rescheduled")
	changed.TimeUpdated = text("2000-01-01T00:00:00Z")
	held, e := observer(t, nil).Observe(r, normalized(t, changed), false, lookups(db, otherDB), clock())
	if e != nil {
		t.Fatal(e)
	}
	if len(held.Snapshot().Revisions) != 2 || held.Snapshot().HoldReason != delivery.ReasonSourceChanged {
		t.Fatal("distinct revision not retained for review")
	}
	accepted, e := held.Finish(inflight, 202, clock(), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range accepted.Snapshot().Batches {
		if b.State != delivery.APIAccepted {
			t.Fatal("inflight receipt lost")
		}
	}
	for _, mode := range []string{"response", "expiry"} {
		var next Record
		if mode == "response" {
			next, e = held.Finish(inflight, 503, clock(), clock())
		} else {
			next, e = held.Recover(clock().Add(time.Minute))
		}
		if e != nil {
			t.Fatal(e)
		}
		for _, b := range next.Snapshot().Batches {
			if b.Message.BatchID == inflight.BatchID && b.State != delivery.ReviewRequired {
				t.Fatal("held attempt became retryable")
			}
		}
	}
	bad, e := held.Prepare(clock(), preparations(t, changed, db)...)
	rejected(t, bad, e)
	bad, _, e = held.Begin(inflight.BatchID, delivery.AttemptContext{Now: clock()})
	rejected(t, bad, e)
	repeat, e := observer(t, nil).Observe(held, normalized(t, src), false, lookups(db), clock())
	if e != nil || repeat.Snapshot().HoldReason == "" {
		t.Fatal("old revision cleared review")
	}
}

func TestGenerationChangesAndExplicitHoldsStopPendingWork(t *testing.T) {
	r := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	o := observer(t, func(v map[string]any) { v["pagerDutyRegion"] = "EU" })
	n, e := o.Observe(r, normalized(t, source(db)), false, lookups(db), clock())
	if e != nil {
		t.Fatal(e)
	}
	if n.Snapshot().Batches[0].State != delivery.ReviewRequired || n.Snapshot().Batches[0].Reason != delivery.ReasonConfigurationChanged {
		t.Fatal("changed generation remained eligible")
	}
	for _, reason := range []delivery.Reason{delivery.ReasonOperator, delivery.ReasonSourceUnavailable, delivery.ReasonSecretUnavailable} {
		n, e = r.Hold(reason, clock())
		if e != nil || n.Snapshot().Batches[0].State != delivery.ReviewRequired || n.Snapshot().HoldReason != reason {
			t.Fatal("explicit hold lost")
		}
	}
}
