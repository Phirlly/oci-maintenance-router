package reconciliation

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLeaseCreateCoalesceAndExpiredTakeover(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	m := leaseManager(t, s, now)
	ctx := leaseBudget(t)
	first := acquireLease(t, m, ctx)
	if err := first.Check(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if s.version != 1 || s.calls[0].method != "CREATE" || s.calls[0].name != leaseName {
		t.Fatal("not conditional create of private lease")
	}
	fields := leaseFields(t, s.body)
	owner, ok := fields["ownerToken"].(string)
	if !ok || len(owner) != 64 || fields["environmentId"] != strings.Repeat("a", 64) || fields["expiresAt"] != now.Add(600*time.Second).Format("2006-01-02T15:04:05.000Z") {
		t.Fatal("wrong ownership record")
	}
	prior := append([]byte(nil), s.body...)
	l, err := m.Acquire(ctx)
	if err != nil || l != nil || !bytes.Equal(prior, s.body) || s.version != 1 {
		t.Fatal("busy lease mutated or claimed", err)
	}
	m.now = func() time.Time { return now.Add(600 * time.Second) }
	second := acquireLease(t, m, ctx)
	if s.version != 2 || bytes.Equal(prior, s.body) {
		t.Fatal("expired owner not replaced")
	}
	if got := leaseFields(t, s.body)["ownerToken"]; got == owner {
		t.Fatal("owner reused")
	}
	if err := second.Check(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(ctx); err == nil {
		t.Fatal("stale owner released replacement")
	}
	for _, call := range s.calls {
		if call.name != leaseName {
			t.Fatal("wrong object")
		}
		if call.method == "REPLACE" && call.etag != "v1" {
			t.Fatal("wrong takeover version")
		}
	}
}

func TestCompetingLeaseAcquisitionsHaveOneWinner(t *testing.T) {
	for _, expired := range []bool{false, true} {
		now := leaseTime()
		s := &leaseMemory{}
		if expired {
			seedLease(s, now.Add(-600*time.Second))
		}
		m := leaseManager(t, s, now)
		ctx := leaseBudget(t)
		var wg sync.WaitGroup
		results := make(chan *Lease, 12)
		failures := make(chan error, 12)
		for range 12 {
			wg.Go(func() { l, e := m.Acquire(ctx); results <- l; failures <- e })
		}
		wg.Wait()
		close(results)
		close(failures)
		winners := 0
		for l := range results {
			if l != nil {
				winners++
			}
		}
		for err := range failures {
			if err != nil {
				leaseFailure(t, err, "conflict")
			}
		}
		if winners != 1 {
			t.Fatalf("winners=%d", winners)
		}
		want := 1
		if expired {
			want = 2
		}
		if s.version != want {
			t.Fatal("extra ownership writes")
		}
	}
}

func TestLeaseTakeoverConflictNeverRefreshesVersionAndRetries(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	seedLease(s, now.Add(-600*time.Second))
	f := leaseFunctionsFor(s)
	calls := 0
	f.replace = func(ctx context.Context, n, e string, b []byte) (string, error) {
		calls++
		_, _ = s.Replace(ctx, n, e, leaseBody(strings.Repeat("a", 64), strings.Repeat("c", 64), now, now.Add(600*time.Second)))
		return s.Replace(ctx, n, e, b)
	}
	l, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
	leaseFailure(t, err, "conflict")
	if l != nil || calls != 1 || s.version != 2 || leaseFields(t, s.body)["ownerToken"] != strings.Repeat("c", 64) {
		t.Fatal("stale takeover retried")
	}
}
