package noticeledger

import (
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func TestReloadRejectsPendingDeliveryUnderUnsupportedAssessment(t *testing.T) {
	prepared := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	sending, ref := begin(t, prepared, prepared.Snapshot().Batches[0].Message.BatchID, clock())
	retry, e := sending.Finish(ref, 503, clock(), clock())
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range []Record{prepared, retry} {
		t.Run(string(r.Snapshot().Batches[0].State), func(t *testing.T) {
			data := changedWire(t, r, func(w *wireRecord) {
				w.Revisions[0].Assessment = routing.ReviewRequired
				w.Revisions[0].AssessmentReason = routing.ReasonType
			})
			env, id := r.Identity()
			n, e := Decode(data, env, id)
			rejected(t, n, e)
		})
	}
}

func TestReloadRetainsHistoryWhenCurrentPolicyNoLongerSupportsSource(t *testing.T) {
	prepared := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	sending, ref := begin(t, prepared, prepared.Snapshot().Batches[0].Message.BatchID, clock())
	accepted, e := sending.Finish(ref, 202, clock(), time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	c := catalog()
	c.MySQLServices = append(c.MySQLServices, "alternate MySQL")
	o, e := New(settings(func(v map[string]any) { v["mysqlServices"] = []string{"alternate MySQL"} }), c, location())
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range []Record{prepared, sending, accepted} {
		want := r.Snapshot().Batches[0].State
		if want == delivery.Prepared {
			want = delivery.ReviewRequired
		}
		n, e := o.Observe(r, normalized(t, source(db)), false, lookups(db), clock())
		if e != nil {
			t.Fatal(e)
		}
		saved := reload(t, n).Snapshot()
		if saved.Revisions[0].Assessment != routing.ReviewRequired || saved.Batches[0].State != want {
			t.Fatal("unsupported assessment erased history or kept pending work")
		}
	}
}

func TestReloadedUnsupportedAttemptRetainsAcceptanceButCannotRetry(t *testing.T) {
	prepared := prepare(t, observed(t, source(db), db), preparations(t, source(db), db)...)
	sending, ref := begin(t, prepared, prepared.Snapshot().Batches[0].Message.BatchID, clock())
	data := changedWire(t, sending, func(w *wireRecord) {
		w.Revisions[0].Assessment = routing.ReviewRequired
		w.Revisions[0].AssessmentReason = routing.ReasonType
	})
	env, id := sending.Identity()
	loaded, e := Decode(data, env, id)
	if e != nil {
		t.Fatal(e)
	}
	for _, outcome := range []string{"accepted", "transient", "expired"} {
		t.Run(outcome, func(t *testing.T) {
			var next Record
			var e error
			want := delivery.ReviewRequired
			switch outcome {
			case "accepted":
				want = delivery.APIAccepted
				next, e = loaded.Finish(ref, 202, clock(), time.Time{})
			case "transient":
				next, e = loaded.Finish(ref, 503, clock(), clock())
			case "expired":
				next, e = loaded.Recover(clock().Add(time.Minute))
			}
			if e != nil {
				t.Fatal(e)
			}
			batch := next.Snapshot().Batches[0]
			if batch.State != want || (want == delivery.ReviewRequired && batch.Reason != delivery.ReasonSourceChanged) {
				t.Fatalf("unsupported outcome %s became %s/%s", outcome, batch.State, batch.Reason)
			}
			if outcome == "expired" && !batch.Ambiguous {
				t.Fatal("expired attempt lost ambiguity")
			}
			reload(t, next)
			bad, _, e := next.Begin(batch.Message.BatchID, delivery.AttemptContext{Now: clock().Add(time.Minute), LeaseExpiresAt: clock().Add(2 * time.Minute), OwnerToken: "worker", RoutingGeneration: batch.Message.RoutingGeneration})
			rejected(t, bad, e)
		})
	}
}
