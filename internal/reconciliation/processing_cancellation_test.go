package reconciliation

import (
	"context"
	"errors"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
	"strings"
	"testing"
	"time"
)

func TestProcessingOwnershipCancellationPreservesContextIdentity(t *testing.T) {
	n := processingNotice(t, 1, processingDB)
	f := processingFixtureFor(t, n)
	f.seedBatch(t, n, time.Now().Add(-time.Minute), delivery.Prepared, false)
	ctx, cancel := context.WithCancel(leaseBudget(t))
	defer cancel()
	r := processingAttemptRun(t, f, ctx, strings.Repeat("e", 64))
	saved, err := r.store.Load(r.ctx, n.Key().NoticeID)
	if err != nil {
		t.Fatal(err)
	}
	f.p.mysql["us-ashburn-1"] = func(context.Context, string) (routing.Metadata, error) {
		cancel()
		return routing.Metadata{}, context.Canceled
	}
	_, _, _, _, err = r.fresh(processingNoticeWork{id: n.Key().NoticeID, member: CandidateRevision{Revision: n.Key()}}, saved)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("ownership cancellation identity lost", err)
	}
}

func TestProcessingPacingPreservesCanceledAndExpiredContextIdentity(t *testing.T) {
	f := processingFixtureFor(t)
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(leaseBudget(t))
		want := context.Canceled
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		r := &processingRun{owner: f.p, ctx: ctx, lease: f.live.lease, started: true}
		if err := r.before(); !errors.Is(err, want) {
			t.Fatal("pacing context identity lost", err)
		}
		cancel()
	}
}
