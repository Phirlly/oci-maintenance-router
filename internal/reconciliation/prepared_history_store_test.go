package reconciliation

import (
	"bytes"
	"testing"
)

func TestPreparedHistoryStoreConditionalLifecycleAndMissingLoad(t *testing.T) {
	store, objects, initial, expanded := preparedStoreFixture(t)
	ctx := leaseBudget(t)
	missing, err := store.Load(ctx)
	preparedStoredFailure(t, missing, err, "read_failed", 404, false)
	if len(objects.calls) != 1 || objects.calls[0].method != "GET" || len(objects.objects) != 0 {
		t.Fatal("missing load initialized history")
	}
	created, err := store.Create(ctx)
	if err != nil || created.History().BaselineID() != "" || created.History().EnvironmentID() != initial.EnvironmentID() {
		t.Fatal("empty creation failed", err)
	}
	first := advancePrepared(t, created.History(), Baseline{}, initial)
	saved, err := store.Save(ctx, created, first)
	if err != nil || saved.History().BaselineID() != initial.ID() {
		t.Fatal("first save failed", err)
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := advancePrepared(t, loaded.History(), initial, expanded)
	final, err := store.Save(ctx, loaded, next)
	if err != nil || final.History().BaselineID() != expanded.ID() {
		t.Fatal("cumulative save failed", err)
	}
	if len(objects.calls) != 5 || objects.calls[1].method != "CREATE" || objects.calls[2].method != "REPLACE" || objects.calls[2].tag != "v1" || objects.calls[4].tag != "v2" {
		t.Fatal("conditional lifecycle changed")
	}
	for _, call := range objects.calls {
		if call.name != "control/prepared-history.json" {
			t.Fatal("unexpected object path")
		}
	}
	want, _ := next.Encode()
	if !bytes.Equal(objects.objects["control/prepared-history.json"].Body, want) || created.History().BaselineID() != "" {
		t.Fatal("stored bytes or original handle mutated")
	}
}
