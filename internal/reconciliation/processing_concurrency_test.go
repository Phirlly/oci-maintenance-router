package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/ocistorage"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMaintenanceConcurrentRunsDoNotReplayAnAcceptedBatch(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	if f.run(t).Accepted != 1 {
		t.Fatal("fixture lacks confirmed acceptance")
	}
	receipt := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, _ = f.p.Run(leaseBudget(t), f.live.lease) })
	}
	wg.Wait()
	// Concurrent observation writes may conflict, but the established receipt
	// remains terminal through both processing and restart.
	f.run(t)
	after := f.saved(t, n.Key().NoticeID).Snapshot().Batches[0]
	if f.sent.Load() != 1 || after.State != delivery.APIAccepted || !reflect.DeepEqual(receipt, after) {
		t.Fatalf("confirmed receipt changed: sent=%d state=%s", f.sent.Load(), after.State)
	}
}

func TestMaintenanceAmbiguousCompetingClaimCannotConfirmAnotherRun(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
	now := time.Now().UTC().Truncate(time.Millisecond)
	f.live.lease.manager.now = func() time.Time { return now }
	ctx := leaseBudget(t)
	a := processingAttemptRun(t, f, ctx, strings.Repeat("d", 64))
	b := processingAttemptRun(t, f, ctx, strings.Repeat("e", 64))
	aSaved, err := a.store.Load(ctx, n.Key().NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	bSaved, err := b.store.Load(ctx, n.Key().NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	ready, written := make(chan struct{}), make(chan struct{})
	f.live.objects.before = func(c context.Context, call discoveryCall) error {
		if processingWriteState(t, f, call) != delivery.Sending {
			return nil
		}
		if writes.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-c.Done():
			return c.Err()
		}
		if strings.Contains(string(call.body), b.token) {
			select {
			case <-written:
				return &ocistorage.Error{Code: ocistorage.Transport, WriteUncertain: true}
			case <-c.Done():
				return c.Err()
			}
		}
		return nil
	}
	f.live.objects.after = func(_ context.Context, call discoveryCall) error {
		if processingWriteState(t, f, call) == delivery.Sending {
			close(written)
		}
		return nil
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		_, e := a.claimAndSend(aSaved, aSaved.Record().Snapshot().Batches[0], []byte(strings.Repeat("k", 32)))
		results <- e
	})
	wg.Go(func() {
		_, e := b.claimAndSend(bSaved, bSaved.Record().Snapshot().Batches[0], []byte(strings.Repeat("k", 32)))
		results <- e
	})
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if own, ok := e.(*ProcessingError); !ok || own.Code != "confirmation_mismatch" {
			t.Fatal("wrong competing-write result", e)
		}
	}
	if success != 1 || f.sent.Load() != 1 || writes.Load() != 2 {
		t.Fatal("ambiguous loser confirmed another attempt")
	}
}
