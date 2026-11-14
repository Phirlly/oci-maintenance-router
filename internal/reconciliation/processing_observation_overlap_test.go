package reconciliation

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/pagerduty"
)

func TestMaintenanceOverlappingObservationPreservesUnconfirmedAttemptWithoutReplay(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var offset atomic.Int64
	now := func() time.Time { return time.Now().Add(time.Duration(offset.Load()) * time.Millisecond) }
	f.live.lease.manager.now, f.p.discovery.now = now, now
	entered, secondDone, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var enter, release sync.Once
	sender := f.p.senders["US"]
	f.p.senders["US"] = func(call context.Context, body, key []byte) (pagerduty.Response, error) {
		enter.Do(func() { close(entered) })
		select {
		case <-secondDone:
			return sender(call, body, key)
		case <-call.Done():
			return pagerduty.Response{}, call.Err()
		}
	}
	var first ProcessingResult
	var firstErr error
	go func() {
		defer close(finished)
		first, firstErr = f.p.Run(ctx, f.live.lease)
	}()
	defer func() {
		cancel()
		release.Do(func() { close(secondDone) })
		<-finished
	}()
	select {
	case <-entered:
	case <-finished:
		t.Fatal("first run did not reach transport", firstErr)
	case <-ctx.Done():
		t.Fatal("transport barrier exceeded original budget", ctx.Err())
	}
	claim := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if claim.State != delivery.Sending || claim.AttemptCount != 1 {
		t.Fatal("first run lacks an exact saved claim")
	}
	// Distinct observation times force an ETag change without wall-clock sleeps.
	offset.Store(1000)
	second, err := f.p.Run(ctx, f.live.lease)
	release.Do(func() { close(secondDone) })
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("first run exceeded original budget", ctx.Err())
	}
	if err != nil || !second.Snapshot().Completed || second.Snapshot().Deferred != 1 || len(second.Snapshot().Attempts) != 0 {
		t.Fatal("overlapping observation did not defer the active claim", err)
	}
	detail, ok := firstErr.(*ProcessingError)
	s := first.Snapshot()
	if !ok || detail.Code != "outcome_unconfirmed" || !detail.WriteUncertain || s.Completed || s.Accepted != 0 || len(s.Attempts) != 1 || s.Attempts[0].Status != 202 || s.Attempts[0].Confirmed || !s.Attempts[0].MayHaveSent {
		t.Fatal("stale outcome was not reported as unconfirmed", firstErr, s)
	}
	if !reflect.DeepEqual(claim, f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]) || f.sent.Load() != 1 {
		t.Fatal("overlap changed the saved claim or duplicated delivery")
	}
	restarted, err := f.p.Run(ctx, f.live.lease)
	if err != nil || !restarted.Snapshot().Completed || restarted.Snapshot().Deferred != 1 || len(restarted.Snapshot().Attempts) != 0 || f.sent.Load() != 1 || !reflect.DeepEqual(claim, f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]) {
		t.Fatal("restart replayed or rewrote an active unconfirmed attempt", err)
	}
}
