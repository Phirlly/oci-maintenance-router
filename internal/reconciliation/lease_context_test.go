package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseDoesNotReviveCanceledOriginalRun(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	m := leaseManager(t, s, now)
	origin, cancel := context.WithCancel(leaseBudget(t))
	l := acquireLease(t, m, origin)
	cancel()
	before := len(s.calls)
	err := l.Check(leaseBudget(t), time.Second)
	if !errors.Is(err, context.Canceled) || len(s.calls) != before {
		t.Fatal("fresh context revived canceled run", err)
	}
}

func TestLeaseOriginalCancellationStopsInflightCheck(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	f := leaseFunctionsFor(s)
	origin, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.get = func(c context.Context, n string) (ocistorage.Object, error) {
		cancel()
		select {
		case <-c.Done():
			return s.Get(c, n)
		case <-time.After(time.Second):
			t.Fatal("original cancellation not propagated")
			return ocistorage.Object{}, nil
		}
	}
	l := acquireLease(t, leaseManager(t, f, now), origin)
	err := l.Check(leaseBudget(t), time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("canceled successful read accepted", err)
	}
}

func TestLeaseChecksOriginalAndCallerTimeBudgetsBeforeAndAfterRead(t *testing.T) {
	for _, late := range []bool{false, true} {
		now := leaseTime()
		s := &leaseMemory{}
		m := leaseManager(t, s, now)
		f := leaseFunctionsFor(s)
		if late {
			f.get = func(c context.Context, n string) (ocistorage.Object, error) {
				m.now = func() time.Time { return now.Add(239 * time.Second) }
				return s.Get(c, n)
			}
			m.objects = f
		}
		l := acquireLease(t, m, leaseBudget(t))
		if !late {
			m.now = func() time.Time { return now.Add(239 * time.Second) }
		}
		leaseFailure(t, l.Check(leaseBudget(t), 2*time.Second), "insufficient_time")
		if !late && len(s.calls) != 1 {
			t.Fatal("read after insufficient budget")
		}
	}
	now := leaseTime()
	s := &leaseMemory{}
	m := leaseManager(t, s, now)
	l := acquireLease(t, m, leaseBudget(t))
	short, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	leaseFailure(t, l.Check(short, time.Second), "insufficient_time")
	if err := l.Check(context.Background(), time.Second); err == nil {
		t.Fatal("unbounded caller accepted")
	}
	for _, duration := range []time.Duration{0, -time.Second} {
		leaseFailure(t, l.Check(leaseBudget(t), duration), "invalid_request")
	}
	m.now = func() time.Time { return now.Add(-time.Second) }
	leaseFailure(t, l.Check(leaseBudget(t), time.Second), "clock_invalid")
	m.now = func() time.Time { return now.Add(240 * time.Second) }
	err := l.Check(leaseBudget(t), time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("local budget revived", err)
	}
}

func TestLeaseAcquireRejectsCanceledSuccessAndCapsOriginalDeadline(t *testing.T) {
	now := leaseTime()
	for _, duration := range []time.Duration{time.Minute, time.Hour} {
		s := &leaseMemory{}
		f := leaseFunctionsFor(s)
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		var observed time.Time
		f.create = func(c context.Context, n string, b []byte) (string, error) {
			observed, _ = c.Deadline()
			tag, e := s.Create(c, n, b)
			cancel()
			return tag, e
		}
		l, err := leaseManager(t, f, now).Acquire(ctx)
		if l != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("canceled claim accepted", err)
		}
		expected, _ := ctx.Deadline()
		if duration > 240*time.Second {
			expected = now.Add(240 * time.Second)
		}
		if !observed.Equal(expected) {
			t.Fatal("wrong acquisition budget")
		}
	}
	s := &leaseMemory{}
	c, cancel := context.WithCancel(leaseBudget(t))
	cancel()
	l, err := leaseManager(t, s, now).Acquire(c)
	if l != nil || !errors.Is(err, context.Canceled) || len(s.calls) != 0 {
		t.Fatal("canceled acquisition ran")
	}
}
