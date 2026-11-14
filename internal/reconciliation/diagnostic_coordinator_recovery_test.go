package reconciliation

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"oci-maintenance-router/internal/ocistorage"
)

func TestDiagnosticCoordinatorCompetingRunsDispatchOnlyFreshWinner(t *testing.T) {
	f := coordinatorFixture(t)
	var arrivals atomic.Int32
	ready := make(chan struct{})
	claims := make(chan string, 2)
	f.objects.before = func(ctx context.Context, c discoveryCall) error {
		if c.method != "CREATE" || c.name != diagnosticObjectName {
			return nil
		}
		r, err := DecodeDiagnostic(c.body, f.coordinator.intent.EnvironmentID, "installation-1", "test-1")
		if err != nil {
			return err
		}
		claims <- r.Snapshot().ClaimToken
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
		value DiagnosticResult
		err   error
	}
	out := make(chan result, 2)
	var wg sync.WaitGroup
	ctx := leaseBudget(t)
	for range 2 {
		wg.Go(func() { v, e := f.coordinator.Run(ctx, f.lease); out <- result{v, e} })
	}
	wg.Wait()
	close(out)
	dispatched := 0
	for r := range out {
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.value.Dispatched() {
			dispatched++
		}
	}
	if dispatched != 1 || f.sends.Load() != 1 || f.saved(t).Snapshot().State != DiagnosticAccepted {
		t.Fatal("competing runs dispatched more than one claim")
	}
	if <-claims == <-claims {
		t.Fatal("operation token reused under same lease")
	}
}

func TestDiagnosticCoordinatorCreateWinnerDoesNotGrantThisRunAuthority(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		f := coordinatorFixture(t)
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if c.method == "CREATE" && c.name == diagnosticObjectName {
				r, e := DecodeDiagnostic(c.body, f.coordinator.intent.EnvironmentID, "installation-1", "test-1")
				if e != nil {
					t.Fatal(e)
				}
				s := r.Snapshot()
				s.ClaimToken = strings.Repeat("f", 64)
				r, e = NewDiagnostic(s.Intent, 9, DiagnosticClaim{s.OwnerToken, s.ClaimToken, s.StartedAt, s.ExpiresAt})
				if e != nil {
					t.Fatal(e)
				}
				if terminal {
					r, e = r.Finish(DiagnosticClaimRef{s.OwnerToken, s.ClaimToken}, 202, s.StartedAt)
					if e != nil {
						t.Fatal(e)
					}
				}
				body, _ := r.Encode()
				f.objects.seed(c.name, body)
			}
			return nil
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if err != nil || got.Dispatched() || f.sends.Load() != 0 || got.Record().Snapshot().SecretVersion != 9 || got.Record().Snapshot().ClaimToken != strings.Repeat("f", 64) {
			t.Fatal("other claim authorized send", err)
		}
	}
}

func TestDiagnosticCoordinatorRequiresConfirmedClaimBeforeSend(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		f := coordinatorFixture(t)
		written := false
		f.objects.after = func(_ context.Context, c discoveryCall) error {
			if c.method == "CREATE" && c.name == diagnosticObjectName {
				written = true
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if written && !confirmed && c.method == "GET" && c.name == diagnosticObjectName {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return nil
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if confirmed {
			if err != nil || !got.Dispatched() || f.sends.Load() != 1 {
				t.Fatal("confirmed first claim failed", err)
			}
		} else {
			e := diagnosticRunFailure(t, err, "read_failed")
			if got.Dispatched() || !e.WriteUncertain || f.sends.Load() != 0 {
				t.Fatal("uncertain claim sent")
			}
			f.objects.before = nil
			f.objects.after = nil
			again, err := f.coordinator.Run(leaseBudget(t), f.lease)
			if err != nil || again.Dispatched() || again.Record().Snapshot().State != DiagnosticSending || f.sends.Load() != 0 {
				t.Fatal("unconfirmed claim resumed", err)
			}
		}
		if preparationCalls(f.objects, "CREATE", diagnosticObjectName) != 1 {
			t.Fatal("claim write retried")
		}
	}
}
