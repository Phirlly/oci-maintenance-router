package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLiveSelectorRejectsInvalidInvocationBeforeIO(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	canceled, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	expired, finish := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer finish()
	for _, tc := range []struct {
		ctx  context.Context
		code string
	}{{nil, "invalid_request"}, {context.Background(), "deadline_required"}, {canceled, "canceled"}, {expired, "deadline_exceeded"}} {
		before := len(f.objects.calls)
		b, err := f.selector.Select(tc.ctx, f.lease)
		liveSelectionFailure(t, b, err, tc.code)
		if tc.ctx != nil && tc.ctx.Err() != nil && !errors.Is(err, tc.ctx.Err()) {
			t.Fatal("lost context identity")
		}
		if len(f.objects.calls) != before {
			t.Fatal("invalid context performed I/O")
		}
	}
	for _, s := range []*LiveBaselineSelector{nil, {}} {
		b, err := s.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, "invalid_request")
	}
	b, err := f.selector.Select(leaseBudget(t), nil)
	liveSelectionFailure(t, b, err, "invalid_request")
	other := leaseManager(t, &leaseMemory{}, leaseTime())
	l := acquireLease(t, other, leaseBudget(t))
	b, err = f.selector.Select(leaseBudget(t), l)
	liveSelectionFailure(t, b, err, "environment_mismatch")
}

func TestLiveSelectorUsesOriginalDeadlineAndCleansChildren(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	ctx, cancel := context.WithTimeout(leaseBudget(t), 40*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	var children []context.Context
	f.objects.before = func(c context.Context, call discoveryCall) error {
		d, ok := c.Deadline()
		if !ok || d.After(deadline) {
			t.Fatal("unbounded or renewed lifetime")
		}
		if call.name != workerLeaseName && d.After(time.Now().Add(10*time.Second)) {
			t.Fatal("storage operation exceeded cap")
		}
		children = append(children, c)
		return nil
	}
	b, err := f.selector.Select(ctx, f.lease)
	if err != nil || b.ID() == "" {
		t.Fatal(err)
	}
	if len(children) < 10 {
		t.Fatal("composition not exercised")
	}
	for _, c := range children {
		if c.Err() == nil {
			t.Fatal("child context not released")
		}
	}
}

func TestLiveSelectorOriginalCancellationPreservesConfirmedWriteWithoutResult(t *testing.T) {
	f := liveSelectorFixtureFor(t)
	if err := f.lease.Release(leaseBudget(t)); err != nil {
		t.Fatal(err)
	}
	origin, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.lease = acquireLease(t, f.lease.manager, origin)
	f.objects.after = func(ctx context.Context, c discoveryCall) error {
		if c.method == "CREATE" && c.name == baselineSelectionName {
			cancel()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				t.Fatal("original cancellation did not propagate")
			}
		}
		return nil
	}
	b, err := f.selector.Select(leaseBudget(t), f.lease)
	e := liveSelectionFailure(t, b, err, "canceled")
	if !errors.Is(err, context.Canceled) || !e.WriteUncertain || preparationCalls(f.objects, "CREATE", baselineSelectionName) != 1 {
		t.Fatal("lost persisted-write/cancellation distinction")
	}
}

func TestLiveSelectorReserveStopsHistoryAndConfirmation(t *testing.T) {
	for _, stage := range []string{"history", "confirmation"} {
		f := liveSelectorFixtureFor(t)
		exhausted := false
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if exhausted {
				t.Fatal("dispatch after reserve exhausted")
			}
			if stage == "history" && c.method == "GET" && c.name == baselineSelectionName {
				exhausted = true
				f.lease.manager.now = func() time.Time { return f.lease.deadline.Add(-time.Second) }
			}
			return nil
		}
		f.objects.after = func(_ context.Context, c discoveryCall) error {
			if stage == "confirmation" && c.method == "CREATE" && c.name == baselineSelectionName {
				exhausted = true
				f.lease.manager.now = func() time.Time { return f.lease.deadline.Add(-time.Second) }
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		code := "insufficient_time"
		if stage == "confirmation" {
			code = "read_failed"
		}
		b, err := f.selector.Select(leaseBudget(t), f.lease)
		liveSelectionFailure(t, b, err, code)
		if !exhausted {
			t.Fatal("reserve boundary not exercised")
		}
	}
}
