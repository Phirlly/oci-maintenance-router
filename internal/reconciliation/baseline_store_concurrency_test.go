package reconciliation

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineStoreCompetingCreatesPreserveOneImmutableObject(t *testing.T) {
	for _, same := range []bool{false, true} {
		store, s, first := baselineStoreFixture(t)
		second := first
		if !same {
			second = buildBaseline(t, first.ID(), baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil))
		}
		type result struct {
			baseline Baseline
			err      error
		}
		results := make(chan result, 2)
		ctx := leaseBudget(t)
		var wg sync.WaitGroup
		for _, b := range []Baseline{first, second} {
			wg.Go(func() { got, err := store.Create(ctx, b); results <- result{got, err} })
		}
		wg.Wait()
		close(results)
		accepted := 0
		var saved []byte
		for got := range results {
			if got.err != nil {
				baselineStoredFailure(t, got.baseline, got.err, "identity_conflict", 200, false)
				continue
			}
			accepted++
			saved, _ = got.baseline.Encode()
		}
		want := 1
		if same {
			want = 2
		}
		if accepted != want || len(s.calls) != 3 || !bytes.Equal(saved, s.objects["baselines/initial.json"].Body) || s.version != 1 {
			t.Fatal("concurrent creation overwrote history or claimed conflicting success")
		}
	}
}

func TestBaselineStoreResultsOwnReturnedBodyAndSnapshots(t *testing.T) {
	_, _, b := baselineStoreFixture(t)
	want, _ := b.Encode()
	returned := append([]byte(nil), want...)
	backend := &baselineBackend{get: func(context.Context, string) (ocistorage.Object, error) {
		return ocistorage.Object{Body: returned, ETag: "v1"}, nil
	}}
	got, err := baselineStoreFor(t, backend, b.EnvironmentID()).Load(leaseBudget(t), b.ID())
	if err != nil {
		t.Fatal(err)
	}
	clear(returned)
	layers := got.Layers()
	layers[0].Cells[0].CompartmentID = "changed"
	encoded, _ := got.Encode()
	clear(encoded)
	if actual, _ := got.Encode(); !bytes.Equal(actual, want) {
		t.Fatal("caller mutated stored result")
	}
}
