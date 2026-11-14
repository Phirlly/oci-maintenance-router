package reconciliation

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDiagnosticConcurrentFirstClaimsPreserveOneWinner(t *testing.T) {
	s, objects, _ := diagnosticStoreFixture(t)
	a := diagnosticFixture(t)
	c := diagnosticClaimFixture()
	c.ClaimToken = strings.Repeat("f", 64)
	b, err := NewDiagnostic(diagnosticIntentFixture(), 9, c)
	if err != nil {
		t.Fatal(err)
	}
	var arrivals atomic.Int32
	ready := make(chan struct{})
	objects.before = func(ctx context.Context, call discoveryCall) error {
		if call.method != "CREATE" {
			return nil
		}
		if arrivals.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	type result struct {
		saved StoredDiagnostic
		err   error
	}
	out := make(chan result, 2)
	var wg sync.WaitGroup
	ctx := leaseBudget(t)
	for _, r := range []DiagnosticRecord{a, b} {
		wg.Go(func() { saved, err := s.Create(ctx, r); out <- result{saved, err} })
	}
	wg.Wait()
	close(out)
	token := ""
	for r := range out {
		if r.err != nil {
			t.Fatal(r.err)
		}
		got := r.saved.Record().Snapshot()
		if token == "" {
			token = got.ClaimToken
		}
		if got.ClaimToken != token {
			t.Fatal("competing callers observed different winning claims")
		}
	}
	if objects.version != 1 || (token != a.Snapshot().ClaimToken && token != b.Snapshot().ClaimToken) {
		t.Fatal("first claim overwritten")
	}
}

func TestDiagnosticConcurrentOutcomesCannotOverwriteEachOther(t *testing.T) {
	for _, same := range []bool{false, true} {
		s, objects, _ := diagnosticStoreFixture(t)
		old := diagnosticSaved(t, s)
		a := diagnosticOutcome(t, old, 202)
		b := a
		if !same {
			b = diagnosticOutcome(t, old, 400)
		}
		type result struct {
			saved StoredDiagnostic
			err   error
		}
		out := make(chan result, 2)
		var wg sync.WaitGroup
		ctx := leaseBudget(t)
		for _, r := range []DiagnosticRecord{a, b} {
			wg.Go(func() { saved, err := s.Save(ctx, old, r); out <- result{saved, err} })
		}
		wg.Wait()
		close(out)
		successes := 0
		for r := range out {
			if r.err == nil {
				successes++
			} else {
				diagnosticStoredFailure(t, r.saved, r.err, "identity_conflict", 200, false)
			}
		}
		want := 1
		if same {
			want = 2
		}
		if successes != want || objects.version != 2 {
			t.Fatal("concurrent results replaced terminal outcome")
		}
	}
}
