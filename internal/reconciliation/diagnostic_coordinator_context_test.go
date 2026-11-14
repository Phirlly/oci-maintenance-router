package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
)

func TestDiagnosticCoordinatorSharesOriginalWorkAndReceiptDeadlines(t *testing.T) {
	for _, limit := range []time.Duration{time.Hour, 40 * time.Second} {
		f := coordinatorFixture(t)
		ctx, cancel := context.WithTimeout(leaseBudget(t), limit)
		defer cancel()
		deadline, _ := ctx.Deadline()
		if f.lease.deadline.Before(deadline) {
			deadline = f.lease.deadline
		}
		work := deadline.Add(-10 * time.Second)
		sent := false
		var receiptCtx context.Context
		f.objects.before = func(c context.Context, call discoveryCall) error {
			d, ok := c.Deadline()
			if !ok {
				t.Fatal("unbounded call")
			}
			if sent {
				receiptCtx = c
				if d.After(deadline) || d.After(time.Now().Add(10*time.Second)) {
					t.Fatal("receipt extended original budget")
				}
			} else if call.name == workerLeaseName {
				if !d.Equal(work) {
					t.Fatal("work deadline renewed")
				}
			} else if d.After(work) || d.After(time.Now().Add(10*time.Second)) {
				t.Fatal("storage read exceeded original budget")
			}
			return nil
		}
		old := f.coordinator.current
		f.coordinator.current = func(c context.Context, id string) (diagnosticCredential, error) {
			d, ok := c.Deadline()
			if !ok || d.After(work) || d.After(time.Now().Add(10*time.Second)) {
				t.Fatal("secret unbounded")
			}
			return old(c, id)
		}
		f.coordinator.send = func(c context.Context, _, _ []byte) (pagerduty.Response, error) {
			d, ok := c.Deadline()
			if !ok || !d.Equal(work) {
				t.Fatal("send revived budget")
			}
			sent = true
			return pagerduty.Response{Status: 202}, nil
		}
		got, err := f.coordinator.Run(ctx, f.lease)
		if err != nil || got.Record().Snapshot().State != DiagnosticAccepted || receiptCtx == nil || receiptCtx.Err() == nil {
			t.Fatal("bounded receipt/cleanup failed", err)
		}
	}
}

func TestDiagnosticCoordinatorOriginalCancellationReachesExternalWork(t *testing.T) {
	for _, stage := range []string{"secret", "send"} {
		origin, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		f := coordinatorFixture(t, origin)
		wait := func(c context.Context) {
			cancel()
			select {
			case <-c.Done():
			case <-time.After(time.Second):
				t.Fatal("original cancellation did not reach operation")
			}
		}
		if stage == "secret" {
			f.coordinator.current = func(c context.Context, _ string) (diagnosticCredential, error) { wait(c); return nil, c.Err() }
		} else {
			f.coordinator.send = func(c context.Context, _, _ []byte) (pagerduty.Response, error) {
				wait(c)
				return pagerduty.Response{Status: 202}, nil
			}
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if stage == "secret" {
			e := diagnosticRunFailure(t, err, "canceled")
			if !errors.Is(e, context.Canceled) || got.Dispatched() {
				t.Fatal("cancellation lost")
			}
		} else if err != nil || got.Record().Snapshot().State != DiagnosticAccepted {
			t.Fatal("accepted outcome lost after origin cancellation", err)
		}
	}
}

func TestDiagnosticCoordinatorReserveBlocksLaterStorageAndSend(t *testing.T) {
	for _, stage := range []string{"initial", "history", "claim-confirm", "key"} {
		f := coordinatorFixture(t)
		exhausted := false
		late := func() {
			exhausted = true
			at := f.lease.deadline.Add(-11 * time.Second)
			f.lease.manager.now = func() time.Time { return at }
		}
		if stage == "initial" {
			late()
		}
		f.objects.before = func(_ context.Context, c discoveryCall) error {
			if exhausted {
				t.Fatal("operation dispatched after reserve exhausted")
			}
			if stage == "history" && c.method == "GET" && c.name == diagnosticObjectName {
				late()
			}
			return nil
		}
		if stage == "claim-confirm" {
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if c.method == "CREATE" && c.name == diagnosticObjectName {
					late()
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				return nil
			}
		}
		if stage == "key" {
			f.coordinator.current = func(context.Context, string) (diagnosticCredential, error) {
				return diagnosticMaterial{7, func() []byte { late(); return []byte("synthetic") }}, nil
			}
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		want := "insufficient_time"
		if stage == "claim-confirm" {
			want = "read_failed"
		}
		diagnosticRunFailure(t, err, want)
		if !exhausted || got.Dispatched() || f.sends.Load() != 0 {
			t.Fatal("reserve did not block dispatch")
		}
	}
}
