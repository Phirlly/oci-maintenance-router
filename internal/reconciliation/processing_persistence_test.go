package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
)

func processingWriteState(t *testing.T, f *processingFixture, c discoveryCall) delivery.State {
	t.Helper()
	if c.name != noticeName(noticeID(1)) || c.method != "REPLACE" {
		return ""
	}
	r, err := noticeledger.Decode(c.body, f.live.lease.manager.environmentID, noticeID(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Snapshot().Batches) == 0 {
		return ""
	}
	return r.Snapshot().Batches[0].State
}

func TestMaintenanceExactLostClaimAndOutcomeResponsesConfirmWithoutReplay(t *testing.T) {
	for _, state := range []delivery.State{delivery.Sending, delivery.APIAccepted} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		lost := false
		f.live.objects.after = func(_ context.Context, c discoveryCall) error {
			if !lost && processingWriteState(t, f, c) == state {
				lost = true
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			}
			return nil
		}
		s := f.run(t)
		if !lost || s.Accepted != 1 || len(s.Attempts) != 1 || !s.Attempts[0].Confirmed || f.sent.Load() != 1 {
			t.Fatal("exact confirmation lost acceptance or replayed")
		}
	}
}

func TestMaintenanceRejectedOrUnconfirmedClaimNeverSends(t *testing.T) {
	for _, status := range []int{412, 403, 0} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if processingWriteState(t, f, c) != delivery.Sending {
				return nil
			}
			if status == 412 {
				return &ocistorage.Error{Code: ocistorage.Conflict, Status: 412}
			}
			if status == 403 {
				return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
			}
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if err == nil || r.Snapshot().Completed || len(r.Snapshot().Attempts) != 0 || f.sent.Load() != 0 {
			t.Fatal("unconfirmed claim dispatched")
		}
		if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Prepared {
			t.Fatal("failed claim changed saved work")
		}
	}
}

func TestMaintenanceObservedAcceptanceWithoutPersistenceRemainsUnconfirmed(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.live.objects.before = func(_ context.Context, c discoveryCall) error {
		if processingWriteState(t, f, c) == delivery.APIAccepted {
			return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	s := r.Snapshot()
	if err == nil || s.Completed || s.Accepted != 0 || len(s.Attempts) != 1 || s.Attempts[0].Status != 202 || s.Attempts[0].Confirmed {
		t.Fatal("observed202 reported durable acceptance")
	}
	if f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.Sending {
		t.Fatal("uncertain outcome was fabricated")
	}
	f.live.objects.before = nil
	again := f.run(t)
	if again.Deferred != 1 || f.sent.Load() != 1 {
		t.Fatal("active uncertain claim resent")
	}
}
