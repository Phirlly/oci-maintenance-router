package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
)

func TestLeaseLostWriteResponseRequiresExactConfirmation(t *testing.T) {
	for _, takeover := range []bool{false, true} {
		for _, applied := range []bool{false, true} {
			now := leaseTime()
			s := &leaseMemory{}
			if takeover {
				seedLease(s, now.Add(-600*time.Second))
			}
			f := leaseFunctionsFor(s)
			calls := 0
			lost := &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			if takeover {
				f.replace = func(c context.Context, n, e string, b []byte) (string, error) {
					calls++
					if applied {
						_, _ = s.Replace(c, n, e, b)
					}
					return "", lost
				}
			} else {
				f.create = func(c context.Context, n string, b []byte) (string, error) {
					calls++
					if applied {
						_, _ = s.Create(c, n, b)
					}
					return "", lost
				}
			}
			l, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
			if applied {
				if err != nil || l == nil {
					t.Fatal("confirmed claim lost", err)
				}
			} else {
				if l != nil || err == nil {
					t.Fatal("unconfirmed claim accepted")
				}
			}
			if calls != 1 {
				t.Fatal("write retransmitted")
			}
		}
	}
}

func TestLeaseUncertainClaimCannotTrustOnlyOwnerToken(t *testing.T) {
	now := leaseTime()
	s := &leaseMemory{}
	f := leaseFunctionsFor(s)
	f.create = func(c context.Context, n string, b []byte) (string, error) {
		altered := bytes.Replace(b, []byte(now.Format("2006-01-02T15:04:05.000Z")), []byte(now.Add(-time.Second).Format("2006-01-02T15:04:05.000Z")), 1)
		_, _ = s.Create(c, n, altered)
		return "", &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
	}
	l, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
	if l != nil || err == nil {
		t.Fatal("changed claim accepted")
	}
}

func TestLeaseExplicitWriteRejectionNeverRecoversViaRead(t *testing.T) {
	now := leaseTime()
	for _, status := range []int{400, 401, 403, 404, 409, 429} {
		s := &leaseMemory{}
		f := leaseFunctionsFor(s)
		reads := 0
		f.create = func(c context.Context, n string, b []byte) (string, error) {
			_, _ = s.Create(c, n, b)
			return "", &ocistorage.Error{Code: ocistorage.Service, Status: status, WriteUncertain: true}
		}
		f.get = func(c context.Context, n string) (ocistorage.Object, error) { reads++; return s.Get(c, n) }
		l, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
		leaseFailure(t, err, "write_failed")
		if l != nil || reads != 0 {
			t.Fatal("explicit rejection recovered")
		}
	}
}

func TestLeaseReadErrorsAndMissingConflictObjectNeverGrantOwnership(t *testing.T) {
	now := leaseTime()
	for _, status := range []int{0, 99, 401, 403, 404, 429, 500, 600} {
		s := &leaseMemory{}
		seedLease(s, now)
		f := leaseFunctionsFor(s)
		fault := error(errors.New("private provider detail"))
		if status != 0 {
			fault = &ocistorage.Error{Code: ocistorage.Service, Status: status}
		}
		f.get = func(context.Context, string) (ocistorage.Object, error) { return ocistorage.Object{}, fault }
		l, err := leaseManager(t, f, now).Acquire(leaseBudget(t))
		leaseFailure(t, err, "read_failed")
		var detail *LeaseError
		_ = errors.As(err, &detail)
		want := status
		if want < 100 || want > 599 {
			want = 0
		}
		if l != nil || detail.HTTPStatus != want || errors.Is(err, fault) || strings.Contains(err.Error(), "private") || s.version != 1 {
			t.Fatal("unsafe error/result")
		}
	}
}
