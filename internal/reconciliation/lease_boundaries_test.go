package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseAcquisitionCannotOutliveItsFixedBudget(t *testing.T) {
	for _, stage := range []string{"token", "create", "conflict read"} {
		t.Run(stage, func(t *testing.T) {
			now := leaseTime()
			s := &leaseMemory{}
			f := leaseFunctionsFor(s)
			m := leaseManager(t, f, now)
			expire := func() { m.now = func() time.Time { return now.Add(240 * time.Second) } }
			switch stage {
			case "token":
				m.token = func() string { expire(); return strings.Repeat("b", 64) }
			case "create":
				f.create = func(c context.Context, n string, b []byte) (string, error) {
					tag, e := s.Create(c, n, b)
					expire()
					return tag, e
				}
			case "conflict read":
				seedLease(s, now.Add(-600*time.Second))
				f.get = func(c context.Context, n string) (ocistorage.Object, error) {
					obj, e := s.Get(c, n)
					expire()
					return obj, e
				}
			}
			m.objects = f
			l, e := m.Acquire(leaseBudget(t))
			if l != nil || !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("late acquisition granted", e)
			}
			if stage == "token" && len(s.calls) != 0 {
				t.Fatal("expired acquisition reached storage")
			}
			if stage == "conflict read" && s.version != 1 {
				t.Fatal("expired takeover wrote state")
			}
		})
	}
}

func TestLeaseCheckRequiresReadableExactVersion(t *testing.T) {
	for _, mode := range []string{"unavailable", "empty version", "oversized version", "new version"} {
		t.Run(mode, func(t *testing.T) {
			now := leaseTime()
			s := &leaseMemory{}
			f := leaseFunctionsFor(s)
			f.get = func(c context.Context, n string) (ocistorage.Object, error) {
				obj, e := s.Get(c, n)
				switch mode {
				case "unavailable":
					return ocistorage.Object{}, &ocistorage.Error{Code: ocistorage.Service, Status: 503}
				case "empty version":
					obj.ETag = ""
				case "oversized version":
					obj.ETag = strings.Repeat("v", 1025)
				case "new version":
					obj.ETag = "new-version"
				}
				return obj, e
			}
			l := acquireLease(t, leaseManager(t, f, now), leaseBudget(t))
			e := l.Check(leaseBudget(t), time.Second)
			want := "invalid_response"
			if mode == "unavailable" {
				want = "read_failed"
			}
			if mode == "new version" {
				want = "ownership_lost"
			}
			leaseFailure(t, e, want)
			if s.version != 1 {
				t.Fatal("failed check changed ownership")
			}
		})
	}
}

func TestLeaseReleaseHonorsCleanupContextBeforeAndAfterWrite(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	f := leaseFunctionsFor(s)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	f.replace = func(c context.Context, n, tag string, b []byte) (string, error) {
		v, e := s.Replace(c, n, tag, b)
		cancel()
		return v, e
	}
	l := acquireLease(t, leaseManager(t, f, now), leaseBudget(t))
	if e := l.Release(context.Background()); e == nil || s.version != 1 {
		t.Fatal("unbounded cleanup wrote")
	}
	e := l.Release(ctx)
	if !errors.Is(e, context.Canceled) || s.version != 2 {
		t.Fatal("canceled write reported success", e)
	}
	before := len(s.calls)
	if e = l.Release(ctx); !errors.Is(e, context.Canceled) || len(s.calls) != before {
		t.Fatal("canceled cleanup issued another write", e)
	}
}

func TestLeaseOriginalDeadlineRemainsCancellationIdentity(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	s := &leaseMemory{}
	l, e := leaseManager(t, s, leaseTime()).Acquire(ctx)
	if l != nil || !errors.Is(e, context.DeadlineExceeded) || len(s.calls) != 0 {
		t.Fatal("expired caller not preserved", e)
	}
}
