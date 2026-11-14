package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"testing"
	"time"
)

// Narrow transition tests use genuine lease, selection and Store handles. This
// fixture is not evidence that bypassing Run grants source or delivery authority.
func processingAttemptRun(t *testing.T, f *processingFixture, ctx context.Context, token string) *processingRun {
	t.Helper()
	deadline, _ := ctx.Deadline()
	if f.live.lease.deadline.Before(deadline) {
		deadline = f.live.lease.deadline
	}
	if limit := time.Now().Add(120 * time.Second); limit.Before(deadline) {
		deadline = limit
	}
	work, cancel := context.WithDeadline(ctx, deadline.Add(-discoveryReserve))
	t.Cleanup(cancel)
	r := &processingRun{owner: f.p, ctx: work, lease: f.live.lease, deadline: deadline, token: token}
	r.storage = &discoveryStorage{objects: f.live.objects, reserve: r.reserve}
	var err error
	r.store, err = ocistorage.NewNoticeStore(r.storage, f.live.lease.manager.environmentID)
	if err != nil {
		t.Fatal(err)
	}
	r.baseline, err = f.p.selector.Select(work, f.live.lease)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
