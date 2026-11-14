package delivery_test

import (
	"oci-maintenance-router/internal/delivery"
	"testing"
	"time"
)

func manualReady(t *testing.T, prior bool) delivery.Record {
	t.Helper()
	r, _ := prepared(t, db)
	if prior {
		s, ref := sending(t)
		var err error
		r, err = s.Finish(ref, 0, instant(), instant())
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := r.Hold(r.Batches()[0].Message.BatchID, delivery.ReasonOperator, instant())
	if err != nil {
		t.Fatal(err)
	}
	return upgraded(t, r)
}

func manualContext(r delivery.Record, at time.Time) delivery.AttemptContext {
	return delivery.AttemptContext{OwnerToken: "manual-worker", RoutingGeneration: r.Batches()[0].Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)}
}

func manualSending(t *testing.T, prior bool) (delivery.Record, delivery.AttemptRef, time.Time) {
	t.Helper()
	r := manualReady(t, prior)
	at := instant().Add(48 * time.Hour)
	next, ref, err := r.BeginManual(r.Batches()[0].Message.BatchID, manualContext(r, at))
	if err != nil {
		t.Fatal(err)
	}
	return next, ref, at
}
