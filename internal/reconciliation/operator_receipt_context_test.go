package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOperatorReceiptUsesOriginalDeadlineAndReleasesReadContexts(t *testing.T) {
	for _, limit := range []time.Duration{time.Hour, 40 * time.Second} {
		f := operatorCoordinatorFixtureFor(t)
		v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
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
		old := v.read
		v.read = func(c context.Context) (OperatorChannel, error) {
			contexts = append(contexts, c)
			d, ok := c.Deadline()
			if !ok || d.After(deadline) || d.After(time.Now().Add(10*time.Second)) {
				t.Fatal("channel exceeded original bounded lifetime")
			}
			return old(c)
		}
		got, err := v.Verify(ctx, f.lease)
		if err != nil || got.Record().Snapshot().State != OperatorAccepted {
			t.Fatal(err)
		}
		for _, c := range contexts {
			if c.Err() == nil {
				t.Fatal("read context not cleaned up")
			}
		}
	}
}

func TestOperatorReceiptCancellationReachesReadAndReturnsNoEvidence(t *testing.T) {
	for _, stage := range []string{"initial", "challenge", "channel", "final"} {
		for _, source := range []string{"caller", "origin"} {
			t.Run(stage+"/"+source, func(t *testing.T) {
				origin, stopOrigin := context.WithCancel(leaseBudget(t))
				defer stopOrigin()
				f := operatorCoordinatorFixtureFor(t, origin)
				v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
				ctx, stopCaller := context.WithCancel(leaseBudget(t))
				defer stopCaller()
				cancel := stopCaller
				if source == "origin" {
					cancel = stopOrigin
				}
				if stage == "initial" {
					cancel()
				}
				if stage == "channel" {
					old := v.read
					v.read = func(c context.Context) (OperatorChannel, error) {
						got, err := old(c)
						cancel()
						select {
						case <-c.Done():
						case <-time.After(time.Second):
							t.Fatal("channel cancellation lost")
						}
						return got, err
					}
				}
				catalogReads := 0
				f.objects.after = func(c context.Context, call discoveryCall) error {
					if call.name == "configuration/catalog.json" {
						catalogReads++
					}
					if stage == "challenge" && call.name == operatorRunObjectName || stage == "final" && call.name == "configuration/catalog.json" && catalogReads == 2 {
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
				e := operatorReceiptFailure(t, got, err, "canceled")
				if !errors.Is(e, context.Canceled) {
					t.Fatal("lost cancellation classification")
				}
				operatorReceiptReadOnly(t, f, before)
			})
		}
	}
}

func TestOperatorReceiptRejectsExpiredOrInsufficientOriginalBudget(t *testing.T) {
	for _, stage := range []string{"initial", "after-challenge", "expired-caller"} {
		f := operatorCoordinatorFixtureFor(t)
		v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, f.seed(t, OperatorAccepted, false)))
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
			if stage == "after-challenge" && c.name == operatorRunObjectName {
				exhaust()
			}
			return nil
		}
		before := len(f.objects.calls)
		got, err := v.Verify(ctx, f.lease)
		operatorReceiptFailure(t, got, err, want)
		operatorReceiptReadOnly(t, f, before)
	}
}

func TestOperatorReceiptRejectsAcceptanceInTheFuture(t *testing.T) {
	f := operatorCoordinatorFixtureFor(t)
	s := f.seed(t, OperatorAccepted, false).Snapshot()
	r, err := NewOperatorChallenge(s.Intent, OperatorClaim{s.OwnerToken, s.ClaimToken, s.StartedAt, time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Finish(OperatorClaimRef{s.OwnerToken, s.ClaimToken}, 200, "message-future", time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := r.Encode()
	f.objects.seed(operatorRunObjectName, body)
	v := operatorReceiptVerifier(t, f, operatorReceiptConfig(t, r))
	got, err := v.Verify(leaseBudget(t), f.lease)
	operatorReceiptFailure(t, got, err, "invalid_time")
}
