package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestPreparationBudgetIsSharedAcrossDiscoveryAndPersistence(t *testing.T) {
	for _, duration := range []time.Duration{time.Hour, 40 * time.Second} {
		p, s, l := preparationFixture(t, nil)
		if err := p.Initialize(leaseBudget(t), l); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(leaseBudget(t), duration)
		defer cancel()
		start := time.Now()
		checks := 0
		var deadline time.Time
		s.before = func(ctx context.Context, c discoveryCall) error {
			seen, ok := ctx.Deadline()
			if !ok {
				t.Fatal("unbounded dispatch")
			}
			if c.name == workerLeaseName {
				if checks == 0 {
					deadline = seen
					limit := duration
					if limit > 120*time.Second {
						limit = 120 * time.Second
					}
					if seen.After(start.Add(limit+time.Second)) || seen.Before(start.Add(limit-time.Second)) {
						t.Fatal("wrong preparation cap")
					}
				} else if seen != deadline {
					t.Fatal("discovery renewed preparation deadline")
				}
				checks++
			} else if seen.After(time.Now().Add(10 * time.Second)) {
				t.Fatal("storage dispatch exceeds operation cap")
			}
			return nil
		}
		b, err := p.Prepare(ctx, l)
		if err != nil || b.ID() == "" {
			t.Fatal("bounded preparation failed", err)
		}
		if checks < 6 {
			t.Fatal("did not exercise discovery and durable advancement")
		}
	}
}

func TestPreparationPropagatesOriginalCancellationDuringWrites(t *testing.T) {
	for _, initialize := range []bool{true, false} {
		p, s, oldLease := preparationFixture(t, nil)
		if !initialize {
			if err := p.Initialize(leaseBudget(t), oldLease); err != nil {
				t.Fatal(err)
			}
		}
		if err := oldLease.Release(leaseBudget(t)); err != nil {
			t.Fatal(err)
		}
		origin, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		l := acquireLease(t, oldLease.manager, origin)
		s.after = func(ctx context.Context, c discoveryCall) error {
			if c.method == "CREATE" {
				cancel()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
					t.Fatal("original cancellation not propagated")
				}
			}
			return nil
		}
		var b Baseline
		var err error
		if initialize {
			err = p.Initialize(leaseBudget(t), l)
		} else {
			b, err = p.Prepare(leaseBudget(t), l)
		}
		d := preparationFailed(t, b, err, "canceled")
		if !errors.Is(d, context.Canceled) || !d.WriteUncertain {
			t.Fatal("lost write/cancellation status")
		}
	}
}

func TestPreparationReserveBlocksLateHistoryAndConfirmationDispatch(t *testing.T) {
	for _, stage := range []string{"history", "confirmation", "head"} {
		p, s, l := preparationFixture(t, nil)
		if stage != "history" {
			if err := p.Initialize(leaseBudget(t), l); err != nil {
				t.Fatal(err)
			}
		}
		var deadline time.Time
		exhausted := false
		s.before = func(ctx context.Context, c discoveryCall) error {
			if exhausted {
				t.Fatalf("dispatch after reserve exhausted: %s %s", c.method, c.name)
			}
			if c.name == workerLeaseName {
				deadline, _ = ctx.Deadline()
			}
			if stage == "history" && c.name == preparedHistoryName && c.method == "GET" {
				exhausted = true
				l.manager.now = func() time.Time { return deadline.Add(-time.Second) }
			}
			return nil
		}
		s.after = func(_ context.Context, c discoveryCall) error {
			if stage != "history" && c.method == "CREATE" {
				exhausted = true
				l.manager.now = func() time.Time { return deadline.Add(-time.Second) }
				if stage == "confirmation" {
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
			}
			return nil
		}
		var b Baseline
		var err error
		if stage == "history" {
			err = p.Initialize(leaseBudget(t), l)
		} else {
			b, err = p.Prepare(leaseBudget(t), l)
		}
		code := "insufficient_time"
		if stage == "confirmation" {
			code = "read_failed"
		}
		preparationFailed(t, b, err, code)
		if !exhausted {
			t.Fatal("budget boundary not exercised")
		}
	}
}
