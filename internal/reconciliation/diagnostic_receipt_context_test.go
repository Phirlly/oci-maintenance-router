package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiagnosticReceiptUsesOriginalDeadlineAndReleasesReadContexts(t *testing.T) {
	for _, limit := range []time.Duration{time.Hour, 40 * time.Second} {
		f := coordinatorFixture(t)
		v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
		ctx, cancel := context.WithTimeout(leaseBudget(t), limit)
		defer cancel()
		deadline, _ := ctx.Deadline()
		if f.lease.deadline.Before(deadline) {
			deadline = f.lease.deadline
		}
		var contexts []context.Context
		f.objects.before = func(c context.Context, call discoveryCall) error {
			contexts = append(contexts, c)
			d, ok := c.Deadline()
			if !ok || d.After(deadline) {
				t.Fatal("read extended original deadline")
			}
			if call.name == workerLeaseName {
				if !d.Equal(deadline) {
					t.Fatal("lease did not retain original lifetime")
				}
			} else if d.After(time.Now().Add(10 * time.Second)) {
				t.Fatal("read exceeded operation cap")
			}
			return nil
		}
		got, err := v.Verify(ctx, f.lease)
		if err != nil || got.Record().Snapshot().State != DiagnosticAccepted {
			t.Fatal(err)
		}
		for _, c := range contexts {
			if c.Err() == nil {
				t.Fatal("read context not cleaned up")
			}
		}
	}
}

func TestDiagnosticReceiptCancellationReachesReadAndReturnsNoEvidence(t *testing.T) {
	for _, stage := range []string{"initial", "diagnostic", "final"} {
		for _, source := range []string{"caller", "origin"} {
			t.Run(stage+"/"+source, func(t *testing.T) {
				origin, stopOrigin := context.WithCancel(leaseBudget(t))
				defer stopOrigin()
				f := coordinatorFixture(t, origin)
				v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
				ctx, stopCaller := context.WithCancel(leaseBudget(t))
				defer stopCaller()
				cancel := stopCaller
				if source == "origin" {
					cancel = stopOrigin
				}
				if stage == "initial" {
					cancel()
				}
				catalogReads := 0
				f.objects.after = func(c context.Context, call discoveryCall) error {
					if call.name == "configuration/catalog.json" {
						catalogReads++
					}
					if stage == "diagnostic" && call.name == diagnosticObjectName || stage == "final" && call.name == "configuration/catalog.json" && catalogReads == 2 {
						cancel()
						select {
						case <-c.Done():
						case <-time.After(time.Second):
							t.Fatal("cancellation did not reach read")
						}
					}
					return nil // Late successful adapters cannot produce evidence.
				}
				before := len(f.objects.calls)
				got, err := v.Verify(ctx, f.lease)
				e := receiptFailure(t, got, err, "canceled")
				if !errors.Is(e, context.Canceled) {
					t.Fatal("lost cancellation classification")
				}
				receiptReadOnly(t, f, before)
			})
		}
	}
}

func TestDiagnosticReceiptRejectsExpiredOrInsufficientOriginalBudget(t *testing.T) {
	for _, stage := range []string{"initial", "after-diagnostic", "expired-caller"} {
		f := coordinatorFixture(t)
		v := receiptVerifier(t, f, receiptConfig(t, f.seed(t, DiagnosticAccepted, false)))
		ctx := leaseBudget(t)
		exhausted := false
		exhaust := func() {
			exhausted = true
			at := f.lease.deadline.Add(-5 * time.Second)
			f.lease.manager.now = func() time.Time { return at }
		}
		want := "insufficient_time"
		if stage == "initial" {
			exhaust()
		}
		if stage == "expired-caller" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()
			want = "deadline_exceeded"
		}
		f.objects.before = func(context.Context, discoveryCall) error {
			if exhausted {
				t.Fatal("read dispatched after reserve exhaustion")
			}
			return nil
		}
		f.objects.after = func(_ context.Context, c discoveryCall) error {
			if stage == "after-diagnostic" && c.name == diagnosticObjectName {
				exhaust()
			}
			return nil
		}
		before := len(f.objects.calls)
		got, err := v.Verify(ctx, f.lease)
		receiptFailure(t, got, err, want)
		receiptReadOnly(t, f, before)
	}
}

func TestDiagnosticReceiptRejectsAcceptanceInTheFuture(t *testing.T) {
	f := coordinatorFixture(t)
	s := f.seed(t, DiagnosticAccepted, false).Snapshot()
	r, err := NewDiagnostic(s.Intent, s.SecretVersion, DiagnosticClaim{s.OwnerToken, s.ClaimToken, s.StartedAt, time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Finish(DiagnosticClaimRef{s.OwnerToken, s.ClaimToken}, 202, time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := r.Encode()
	f.objects.seed(diagnosticObjectName, body)
	v := receiptVerifier(t, f, receiptConfig(t, r))
	got, err := v.Verify(leaseBudget(t), f.lease)
	receiptFailure(t, got, err, "invalid_time")
}
