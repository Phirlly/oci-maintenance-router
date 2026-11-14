package reconciliation

import (
	"sync"
	"testing"
)

func TestPreparedHistoryConcurrentSavesPreserveWinner(t *testing.T) {
	for _, same := range []bool{false, true} {
		s, objects, first, _ := preparedStoreFixture(t)
		ctx := leaseBudget(t)
		old, err := s.Create(ctx)
		if err != nil {
			t.Fatal(err)
		}
		a := advancePrepared(t, old.History(), Baseline{}, first)
		b := a
		if !same {
			b = advancePrepared(t, old.History(), Baseline{}, buildBaseline(t, "other", baselineResult(t, nil, nil, []string{baselineOtherCompartment}, nil)))
		}
		type result struct {
			saved StoredPreparedHistory
			err   error
		}
		results := make(chan result, 2)
		var wg sync.WaitGroup
		for _, p := range []PreparedHistory{a, b} {
			wg.Go(func() { got, err := s.Save(ctx, old, p); results <- result{got, err} })
		}
		wg.Wait()
		close(results)
		accepted := 0
		for r := range results {
			if r.err != nil {
				preparedStoredFailure(t, r.saved, r.err, "identity_conflict", 200, false)
			} else {
				accepted++
			}
		}
		want := 1
		if same {
			want = 2
		}
		if accepted != want || objects.version != 2 || len(objects.calls) != 4 {
			t.Fatal("competing save overwrote history or misreported success")
		}
	}
}
