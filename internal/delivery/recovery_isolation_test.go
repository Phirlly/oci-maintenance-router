package delivery

import (
	"reflect"
	"testing"
	"time"
)

func TestManualRecoveryAndClosurePreserveIndependentBatches(t *testing.T) {
	r, _ := codecFixture(t, "first")
	_, second := codecFixture(t, "second")
	r, err := r.Prepare(second)
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.MigrateRecovery()
	if err != nil {
		t.Fatal(err)
	}
	before := r.Batches()
	target, peer := before[0], before[1]
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	held, err := r.Hold(target.Message.BatchID, ReasonOperator, at)
	if err != nil {
		t.Fatal(err)
	}
	s, ref, err := held.BeginManual(target.Message.BatchID, AttemptContext{OwnerToken: "manual", RoutingGeneration: target.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := s.Finish(ref, 503, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := reviewed.Close(target.Message.BatchID, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []Record{r, held, s, reviewed, closed} {
		got, ok := record.Batch(peer.Message.BatchID)
		if !ok || !reflect.DeepEqual(got, peer) {
			t.Fatal("targeted recovery changed peer")
		}
		if _, err := decodeData(record.copy()); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(r.Batches(), before) {
		t.Fatal("transition mutated predecessor")
	}
	sent, peerRef, err := closed.Begin(peer.Message.BatchID, AttemptContext{OwnerToken: "automatic", RoutingGeneration: peer.Message.RoutingGeneration, Now: at, LeaseExpiresAt: at.Add(time.Minute)})
	if err != nil {
		t.Fatal("closed peer blocked independent automatic work", err)
	}
	accepted, err := sent.Finish(peerRef, 202, at, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeData(accepted.copy()); err != nil {
		t.Fatal(err)
	}
}
