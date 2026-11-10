package reconciliation

import (
	"bytes"
	"context"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseReleaseIsConditionalTerminalAndIdempotent(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	m := leaseManager(t, s, now)
	ctx := leaseBudget(t)
	l := acquireLease(t, m, ctx)
	acquired := leaseFields(t, s.body)
	if err := l.Release(ctx); err != nil {
		t.Fatal(err)
	}
	released := leaseFields(t, s.body)
	if s.version != 2 || released["ownerToken"] != acquired["ownerToken"] || released["expiresAt"] != acquired["acquiredAt"] {
		t.Fatal("release changed identity or failed to expire")
	}
	if err := l.Check(ctx, time.Second); err == nil {
		t.Fatal("released handle retained authority")
	}
	if err := l.Release(ctx); err != nil || s.version != 2 {
		t.Fatal("release not idempotent", err)
	}
	replacement := acquireLease(t, m, ctx)
	prior := append([]byte(nil), s.body...)
	leaseFailure(t, l.Release(ctx), "ownership_lost")
	if !bytes.Equal(s.body, prior) || s.version != 3 {
		t.Fatal("old release overwrote replacement")
	}
	if err := replacement.Check(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseLostReleaseResponseNeedsExactTerminalRecord(t *testing.T) {
	for _, applied := range []bool{false, true} {
		now := leaseTime()
		s := &leaseMemory{}
		f := leaseFunctionsFor(s)
		calls := 0
		f.replace = func(c context.Context, n, e string, b []byte) (string, error) {
			calls++
			if applied {
				_, _ = s.Replace(c, n, e, b)
			}
			return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		l := acquireLease(t, leaseManager(t, f, now), leaseBudget(t))
		err := l.Release(leaseBudget(t))
		if (err == nil) != applied || calls != 1 {
			t.Fatal("wrong release confirmation", err)
		}
	}
}

func TestLeaseCleanupUsesOwnBoundedContextAfterWorkCancellation(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	m := leaseManager(t, s, now)
	original, cancel := context.WithCancel(leaseBudget(t))
	l := acquireLease(t, m, original)
	cancel()
	m.now = func() time.Time { return now.Add(241 * time.Second) }
	if err := l.Release(leaseBudget(t)); err != nil || s.version != 2 {
		t.Fatal("bounded cleanup failed", err)
	}
	m.now = func() time.Time { return now.Add(600 * time.Second) }
	if err := l.Release(leaseBudget(t)); err == nil {
		t.Fatal("expired cleanup accepted")
	}
}
