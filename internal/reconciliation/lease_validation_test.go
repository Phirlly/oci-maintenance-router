package reconciliation

import (
	"context"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseRejectsInvalidManagerAndInputsWithoutCalls(t *testing.T) {
	s := &leaseMemory{}
	for _, env := range []string{"", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		m, e := NewLeaseManager(s, env)
		if m != nil || e == nil {
			t.Fatal("invalid environment accepted")
		}
	}
	if m, e := NewLeaseManager(nil, strings.Repeat("a", 64)); m != nil || e == nil {
		t.Fatal("nil storage accepted")
	}
	for _, m := range []*LeaseManager{nil, {}} {
		l, e := m.Acquire(leaseBudget(t))
		if l != nil {
			t.Fatal("zero manager claimed")
		}
		leaseFailure(t, e, "invalid_request")
	}
	m := leaseManager(t, s, leaseTime())
	for _, ctx := range []context.Context{nil, context.Background()} {
		l, e := m.Acquire(ctx)
		if l != nil || e == nil {
			t.Fatal("unbounded context accepted")
		}
	}
	m.token = func() string { return "bad" }
	l, e := m.Acquire(leaseBudget(t))
	if l != nil || e == nil || len(s.calls) != 0 {
		t.Fatal("invalid token reached storage")
	}
	for _, l := range []*Lease{nil, {}} {
		leaseFailure(t, l.Check(leaseBudget(t), time.Second), "invalid_request")
		leaseFailure(t, l.Release(leaseBudget(t)), "invalid_request")
	}
	var absent *ocistorage.Client
	l, e = leaseManager(t, absent, leaseTime()).Acquire(leaseBudget(t))
	if l != nil || e == nil {
		t.Fatal("typed nil adapter claimed")
	}
}

func TestLeaseRejectsInvalidClocksAndEmptyVersions(t *testing.T) {
	for _, now := range []time.Time{{}, time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)} {
		s := &leaseMemory{}
		l, e := leaseManager(t, s, now).Acquire(leaseBudget(t))
		if l != nil || e == nil || len(s.calls) != 0 {
			t.Fatal("invalid clock reached storage")
		}
	}
	now := leaseTime()
	s := &leaseMemory{}
	seedLease(s, now.Add(time.Minute))
	l, e := leaseManager(t, s, now).Acquire(leaseBudget(t))
	leaseFailure(t, e, "clock_invalid")
	if l != nil || s.version != 1 {
		t.Fatal("future owner replaced")
	}
	f := leaseFunctionsFor(&leaseMemory{})
	f.create = func(context.Context, string, []byte) (string, error) { return "", nil }
	l, e = leaseManager(t, f, now).Acquire(leaseBudget(t))
	leaseFailure(t, e, "invalid_response")
	if l != nil {
		t.Fatal("empty ETag claimed")
	}
}
