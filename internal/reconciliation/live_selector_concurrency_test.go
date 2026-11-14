package reconciliation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLiveSelectorConcurrentExactSelectionsConfirmOneValue(t *testing.T) {
	for _, advance := range []bool{false, true} {
		f := liveSelectorFixtureFor(t)
		want := f.baseline
		method := "CREATE"
		if advance {
			f.selectOK(t)
			want = f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
			f.configure(t)
			method = "REPLACE"
		}
		var writes atomic.Int32
		ready := make(chan struct{})
		f.objects.before = func(ctx context.Context, c discoveryCall) error {
			if c.name == baselineSelectionName && c.method == method {
				if writes.Add(1) == 2 {
					close(ready)
				}
				select {
				case <-ready:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}
		type result struct {
			b   Baseline
			err error
		}
		results := make(chan result, 2)
		ctx := leaseBudget(t)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { b, err := f.selector.Select(ctx, f.lease); results <- result{b, err} })
		}
		wg.Wait()
		close(results)
		for r := range results {
			if r.err != nil || r.b.ID() != want.ID() {
				t.Fatal("exact concurrent selection failed", r.err)
			}
		}
		if writes.Load() != 2 {
			t.Fatal("did not exercise conditional conflict")
		}
		f.objects.before = nil
		f.configure(t)
		f.selectOK(t)
	}
}

func TestLiveSelectorConcurrentDifferentTargetsDoNotRebase(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	f.selectOK(t)
	second := f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, nil)
	secondHead := f.objects.objects[preparedHistoryName]
	third := f.extend(t, []string{baselineCompartment, baselineOtherCompartment}, []string{"us-ashburn-1", "us-phoenix-1"})
	thirdHead := f.objects.objects[preparedHistoryName]
	f.configure(t)
	f.objects.seed(preparedHistoryName, secondHead.Body)
	var reads, writes, headRequests atomic.Int32
	ready := make(chan struct{})
	headChanged := make(chan struct{})
	f.objects.after = func(_ context.Context, c discoveryCall) error {
		if c.method == "GET" && c.name == preparedHistoryName && reads.Add(1) == 1 {
			f.objects.seed(preparedHistoryName, thirdHead.Body)
			close(headChanged)
		}
		return nil
	}
	f.objects.before = func(ctx context.Context, c discoveryCall) error {
		// The second GET must observe the new head, rather than racing the first
		// reader between its snapshot and after-hook. Both writes still compete.
		if c.method == "GET" && c.name == preparedHistoryName && headRequests.Add(1) > 1 {
			select {
			case <-headChanged:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if c.method == "REPLACE" && c.name == baselineSelectionName {
			if writes.Add(1) == 2 {
				close(ready)
			}
			select {
			case <-ready:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	type result struct {
		b   Baseline
		err error
	}
	results := make(chan result, 2)
	ctx := leaseBudget(t)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { b, err := f.selector.Select(ctx, f.lease); results <- result{b, err} })
	}
	wg.Wait()
	close(results)
	accepted := 0
	var winner Baseline
	for r := range results {
		if r.err == nil {
			accepted++
			winner = r.b
		} else {
			liveSelectionFailure(t, r.b, r.err, "identity_conflict")
		}
	}
	if accepted != 1 || writes.Load() != 2 || (winner.ID() != second.ID() && winner.ID() != third.ID()) {
		t.Fatal("different targets merged or misreported")
	}
	f.objects.before, f.objects.after = nil, nil
	obj, _ := f.objects.Get(ctx, baselineSelectionName)
	s, err := DecodeBaselineSelection(obj.Body, f.lease.manager.environmentID, "installation-1")
	if err != nil || !s.Matches(winner) {
		t.Fatal("winner overwritten", err)
	}
}
