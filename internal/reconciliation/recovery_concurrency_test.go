package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/routing"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryCompetingManualClaimCannotConfirmAnotherOwner(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	old := f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, true)
	projected, err := old.MigrateRecovery(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := projected.Encode()
	f.live.objects.seed(noticeName(n.Key().NoticeID), body)
	q := reviewedRecovery(t, projected, noticeledger.RetryBatch, projected.Snapshot().Batches[0].Message.BatchID)
	f.configure(t, func(v map[string]any) { v["schemaVersion"], v["recovery"] = 7, q })
	now := time.Now().UTC().Truncate(time.Millisecond)
	f.live.lease.manager.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
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
	metadata, err := f.p.mysql["us-ashburn-1"](ctx, processingDB)
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]routing.Lookup{processingDB: {State: routing.Found, Metadata: metadata}}
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
	wg.Go(func() { results <- a.retryRecovery(aSaved, n, lookups) })
	wg.Go(func() { results <- b.retryRecovery(bSaved, n, lookups) })
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if e, ok := err.(*ProcessingError); !ok || e.Code != "confirmation_mismatch" {
			t.Fatal("unexpected competing result", err)
		}
	}
	if success != 1 || writes.Load() != 2 || f.sent.Load() != 1 || len(recoveryActions(t, f, n.Key().NoticeID)) != 1 {
		t.Fatal("manual action was consumed more than once")
	}
}
