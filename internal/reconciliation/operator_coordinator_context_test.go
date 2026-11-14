package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-maintenance-router/internal/ocinotifications"
	"oci-maintenance-router/internal/ocistorage"
)

func TestOperatorCoordinatorSharesOriginalWorkAndOutcomeDeadlines(t *testing.T) {
	for _, limit := range []time.Duration{time.Hour, 40 * time.Second} {
		f := operatorCoordinatorFixtureFor(t)
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
					t.Fatal("outcome extended original budget")
				}
			} else if call.name == workerLeaseName {
				if !d.Equal(work) {
					t.Fatal("work deadline renewed")
				}
			} else if d.After(work) || d.After(time.Now().Add(10*time.Second)) {
				t.Fatal("storage exceeded original budget")
			}
			return nil
		}
		old := f.coordinator.read
		f.coordinator.read = func(c context.Context) (operatorObservation, error) {
			d, ok := c.Deadline()
			if !ok || d.After(work) || d.After(time.Now().Add(10*time.Second)) {
				t.Fatal("channel read unbounded")
			}
			return old(c)
		}
		f.publish = func(c context.Context, _ []byte) (ocinotifications.Publication, error) {
			d, ok := c.Deadline()
			if !ok || !d.Equal(work) {
				t.Fatal("publication revived budget")
			}
			sent = true
			return ocinotifications.Publication{Status: 200, MessageID: "message-1"}, nil
		}
		got, err := f.coordinator.Run(ctx, f.lease)
		if err != nil || got.Record().Snapshot().State != OperatorAccepted || receiptCtx == nil || receiptCtx.Err() == nil {
			t.Fatal("bounded outcome/cleanup failed", err)
		}
	}
}

func TestOperatorCoordinatorOriginalCancellationReachesExternalWork(t *testing.T) {
	for _, stage := range []string{"channel", "publish"} {
		origin, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		f := operatorCoordinatorFixtureFor(t, origin)
		wait := func(c context.Context) {
			cancel()
			select {
			case <-c.Done():
			case <-time.After(time.Second):
				t.Fatal("original cancellation did not reach operation")
			}
		}
		if stage == "channel" {
			f.coordinator.read = func(c context.Context) (operatorObservation, error) { wait(c); return operatorObservation{}, c.Err() }
		} else {
			f.publish = func(c context.Context, _ []byte) (ocinotifications.Publication, error) {
				wait(c)
				return ocinotifications.Publication{Status: 200, MessageID: "message-1"}, nil
			}
		}
		got, err := f.coordinator.Run(leaseBudget(t), f.lease)
		if stage == "channel" {
			e := operatorRunFailure(t, err, "canceled")
			if !errors.Is(e, context.Canceled) || got.Dispatched() {
				t.Fatal("cancellation lost")
			}
		} else if err != nil || got.Record().Snapshot().State != OperatorAccepted {
			t.Fatal("outcome lost after origin cancellation", err)
		}
	}
}

func TestOperatorCoordinatorReserveBlocksFurtherIO(t *testing.T) {
	for _, stage := range []string{"initial", "history", "create", "claim-confirm", "second-channel", "final-control"} {
		t.Run(stage, func(t *testing.T) {
			f := operatorCoordinatorFixtureFor(t)
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
					t.Fatal("I/O after reserve exhausted")
				}
				if stage == "history" && c.method == "GET" && c.name == operatorRunObjectName {
					late()
				}
				return nil
			}
			f.objects.after = func(_ context.Context, c discoveryCall) error {
				if stage == "create" && c.method == "HISTORY" {
					late()
				}
				if stage == "claim-confirm" && c.method == "CREATE" && c.name == operatorRunObjectName {
					late()
					return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
				}
				if stage == "final-control" && f.reads.Load() == 2 && c.name == "configuration/catalog.json" {
					late()
				}
				return nil
			}
			old := f.coordinator.read
			f.coordinator.read = func(c context.Context) (operatorObservation, error) {
				o, e := old(c)
				if stage == "second-channel" && f.reads.Load() == 2 {
					late()
				}
				return o, e
			}
			got, err := f.coordinator.Run(leaseBudget(t), f.lease)
			want := "insufficient_time"
			if stage == "claim-confirm" {
				want = "read_failed"
			}
			if stage == "create" {
				want = "write_failed"
			}
			operatorRunFailure(t, err, want)
			if !exhausted || got.Dispatched() || f.sends.Load() != 0 {
				t.Fatal("reserve failed to block publication")
			}
		})
	}
}
