package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"testing"
	"time"
)

func TestMaintenanceCanceledWorkOnlyConfirmsTheIssuedOutcome(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	sent := false
	methods := []string{}
	var workDeadline, receiptDeadline time.Time
	f.live.objects.before = func(c context.Context, call discoveryCall) error {
		if sent {
			if call.name != noticeName(n.Key().NoticeID) || (call.method != "REPLACE" && call.method != "GET") || c.Err() != nil {
				t.Fatal("outcome context escaped its boundary")
			}
			methods = append(methods, call.method)
			receiptDeadline, _ = c.Deadline()
		}
		return nil
	}
	f.live.objects.after = func(_ context.Context, call discoveryCall) error {
		if sent && call.method == "REPLACE" {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	f.p.senders["US"] = func(c context.Context, _, _ []byte) (pagerduty.Response, error) {
		workDeadline, _ = c.Deadline()
		sent = true
		cancel()
		return pagerduty.Response{Status: 202}, nil
	}
	r, err := f.p.Run(ctx, f.live.lease)
	s := r.Snapshot()
	if !errors.Is(err, context.Canceled) || s.Completed || s.Accepted != 1 || len(s.Attempts) != 1 || !s.Attempts[0].Confirmed || len(methods) != 2 || methods[0] != "REPLACE" || methods[1] != "GET" {
		t.Fatal("canceled accepted outcome not confirmed exactly", err)
	}
	if receiptDeadline.After(workDeadline.Add(discoveryReserve)) || receiptDeadline.After(f.live.lease.deadline) {
		t.Fatal("receipt extended original/delivery budget")
	}
	f.live.objects.before, f.live.objects.after = nil, nil
	if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.APIAccepted {
		t.Fatal("receipt not durable")
	}
}

func TestMaintenanceControlLossAfterClaimDoesNotDispatch(t *testing.T) {
	for _, change := range []string{"configuration", "lease", "cancel"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		ctx, cancel := context.WithCancel(leaseBudget(t))
		defer cancel()
		changed := false
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if !changed && processingWriteState(t, f, c) == delivery.Sending {
				changed = true
				switch change {
				case "configuration":
					f.live.objects.seed("configuration/router.json", []byte("changed"))
				case "lease":
					f.live.objects.mu.Lock()
					body := f.live.objects.objects[workerLeaseName].Body
					f.live.objects.mu.Unlock()
					f.live.objects.seed(workerLeaseName, body)
				case "cancel":
					cancel()
				}
			}
			return nil
		}
		r, err := f.p.Run(ctx, f.live.lease)
		if !changed || err == nil || len(r.Snapshot().Attempts) != 0 || f.sent.Load() != 0 {
			t.Fatal("lost authority dispatched", err)
		}
		if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Sending {
			t.Fatal("claimed uncertainty erased")
		}
	}
}
