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

func TestMaintenanceWaitsForDueRetryOrRetainsItWhenWaitingFails(t *testing.T) {
	for _, failure := range []string{"none", "wait", "recovery_write"} {
		n := processingNotice(t, 1, processingDB)
		f := processingFixtureFor(t, n)
		now := time.Now().UTC().Truncate(time.Millisecond)
		f.live.lease.manager.now = func() time.Time { return now }
		f.p.discovery.now = f.live.lease.manager.now
		waited := false
		f.p.wait = func(_ context.Context, d time.Duration) error {
			if d != 100*time.Millisecond {
				waited = true
				if failure == "wait" {
					return errors.New("private wait")
				}
			}
			now = now.Add(d)
			return nil
		}
		send := f.p.senders["US"]
		f.p.senders["US"] = func(c context.Context, body, key []byte) (pagerduty.Response, error) {
			response, err := send(c, body, key)
			if f.sent.Load() == 1 {
				response.Status = 503
				response.RetryAfter = now.Add(500 * time.Millisecond)
			}
			return response, err
		}
		writes := 0
		f.live.objects.before = func(_ context.Context, c discoveryCall) error {
			if processingWriteState(t, f, c) == delivery.RetryWait {
				writes++
				if writes == 2 && failure == "recovery_write" {
					return &ocistorage.Error{Code: ocistorage.Service, Status: 403}
				}
			}
			return nil
		}
		r, err := f.p.Run(leaseBudget(t), f.live.lease)
		if !waited {
			t.Fatal("retry delay skipped")
		}
		if failure == "none" {
			if err != nil || r.Snapshot().Accepted != 1 || f.sent.Load() != 2 {
				t.Fatal("due retry not delivered", err)
			}
		} else if err == nil || r.Snapshot().Completed || f.sent.Load() != 1 || f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].State != delivery.RetryWait {
			t.Fatal("failed retry wait/recovery lost pending state", err)
		}
	}
}

func TestMaintenanceOversleptRetryCrossingHorizonBecomesReview(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	now := time.Now().UTC().Truncate(time.Millisecond)
	record := f.seedBatch(t, n, now.Add(-24*time.Hour+time.Second), delivery.RetryWait, false)
	b := record.Snapshot().Batches[0]
	sending, ref, err := record.Begin(b.Message.BatchID, delivery.AttemptContext{OwnerToken: "old", RoutingGeneration: b.Message.RoutingGeneration, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	record, err = sending.Finish(ref, 503, now, now.Add(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := record.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	waited := false
	f.p.wait = func(_ context.Context, d time.Duration) error {
		if d != 100*time.Millisecond {
			waited = true
			f.live.lease.manager.now = func() time.Time { return now.Add(2 * time.Second) }
		}
		return nil
	}
	s := f.run(t)
	if !waited || s.Accepted != 0 || f.sent.Load() != 0 || f.saved(t, n.Key().NoticeID).Snapshot().Batches[0].Reason != delivery.ReasonHorizon {
		t.Fatal("overslept retry ignored horizon")
	}
}

func TestMaintenanceInvalidClockAfterSavedRetryPreventsAnotherAttempt(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	send := f.p.senders["US"]
	f.p.senders["US"] = func(c context.Context, body, key []byte) (pagerduty.Response, error) {
		_, err := send(c, body, key)
		return pagerduty.Response{Status: 503}, err
	}
	f.live.objects.after = func(_ context.Context, c discoveryCall) error {
		if processingWriteState(t, f, c) == delivery.RetryWait {
			f.live.lease.manager.now = func() time.Time { return time.Time{} }
		}
		return nil
	}
	r, err := f.p.Run(leaseBudget(t), f.live.lease)
	if err == nil || r.Snapshot().Completed || f.sent.Load() != 1 || len(r.Snapshot().Attempts) != 1 || !r.Snapshot().Attempts[0].Confirmed {
		t.Fatal("invalid retry clock continued or lost saved outcome")
	}
}
