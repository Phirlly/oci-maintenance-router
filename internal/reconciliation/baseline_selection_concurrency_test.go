package reconciliation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestBaselineSelectionConcurrentSavesPreserveWinner(t *testing.T) {
	for _, same := range []bool{false, true} {
		s, objects, _, first, second := selectionStoreFixture(t)
		ctx := leaseBudget(t)
		old := selectionSaved(t, s, first)
		a := advanceSelection(t, old.Selection(), first, second)
		b := a
		if !same {
			third, err := second.Extend("third", baselineResult(t, nil, nil, nil, []string{"us-phoenix-1"}))
			if err != nil {
				t.Fatal(err)
			}
			b = advanceSelection(t, old.Selection(), first, third)
		}
		type result struct {
			saved StoredBaselineSelection
			err   error
		}
		results := make(chan result, 2)
		var wg sync.WaitGroup
		for _, p := range []BaselineSelection{a, b} {
			wg.Go(func() { got, err := s.Save(ctx, old, p); results <- result{got, err} })
		}
		wg.Wait()
		close(results)
		accepted := 0
		for r := range results {
			if r.err != nil {
				selectionStoredFailure(t, r.saved, r.err, "identity_conflict", 200, false)
			} else {
				accepted++
			}
		}
		want := 1
		if same {
			want = 2
		}
		if accepted != want || objects.version != 2 || len(objects.calls) != 6 {
			t.Fatal("competing writes overwrote winner or misreported success")
		}
	}
}

func TestBaselineSelectionConcurrentFirstCreatesConfirmOnlyExactWinner(t *testing.T) {
	for _, same := range []bool{false, true} {
		s, objects, backend, first, second := selectionStoreFixture(t)
		a := selectionFor(t, first)
		b := a
		if !same {
			b = selectionFor(t, second)
		}
		var reads atomic.Int32
		ready := make(chan struct{})
		backend.get = func(c context.Context, n string) (ocistorage.Object, error) {
			obj, err := objects.Get(c, n)
			count := reads.Add(1)
			if count == 2 {
				close(ready)
			}
			if count <= 2 {
				select {
				case <-ready:
				case <-c.Done():
					return ocistorage.Object{}, c.Err()
				}
			}
			return obj, err
		}
		type result struct {
			saved StoredBaselineSelection
			err   error
		}
		results := make(chan result, 2)
		ctx := leaseBudget(t)
		var wg sync.WaitGroup
		for _, p := range []BaselineSelection{a, b} {
			wg.Go(func() { got, err := s.Create(ctx, p); results <- result{got, err} })
		}
		wg.Wait()
		close(results)
		accepted := 0
		for r := range results {
			if r.err != nil {
				selectionStoredFailure(t, r.saved, r.err, "identity_conflict", 200, false)
			} else {
				accepted++
			}
		}
		want := 1
		if same {
			want = 2
		}
		if accepted != want || objects.version != 1 || len(objects.calls) != 7 {
			t.Fatal("concurrent create replaced state or accepted another winner")
		}
	}
}
