package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/ocistorage"
	"time"
)

type processingRun struct {
	owner     *MaintenanceProcessor
	ctx       context.Context
	lease     *Lease
	deadline  time.Time
	token     string
	store     *ocistorage.NoticeStore
	storage   *discoveryStorage
	baseline  Baseline
	final     DiscoveryResult
	result    ProcessingResult
	textBytes int
	started   bool
}

// Run returns confirmed partial work even on failure. Completed means this pass
// finished, not that every notice was eligible or every notification was received.
func (p *MaintenanceProcessor) Run(ctx context.Context, lease *Lease) (ProcessingResult, error) {
	if p == nil || p.discovery == nil || p.selector == nil || p.current == nil || p.version == nil || p.wait == nil || p.jitter == nil || p.token == nil || !lease.usable() {
		return ProcessingResult{}, &ProcessingError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return ProcessingResult{}, processingFailure(ctx, err)
	}
	if lease.manager.environmentID != p.discovery.collector.environmentID {
		return ProcessingResult{}, &ProcessingError{Code: "environment_mismatch"}
	}
	token := p.token()
	if !leaseDigest.MatchString(token) || token == lease.record.OwnerToken {
		return ProcessingResult{}, &ProcessingError{Code: "invalid_request"}
	}
	work, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	r := &processingRun{owner: p, ctx: work, lease: lease, token: token}
	err := r.run()
	if err != nil {
		if lease.origin.Err() != nil {
			work = lease.origin
		}
		return r.result, processingFailure(work, err)
	}
	return r.result, nil
}

func (r *processingRun) run() error {
	initial, err := r.owner.discovery.Run(r.ctx, r.lease)
	if err != nil {
		return err
	}
	now := r.lease.manager.now()
	if !leaseWallTime(now) || !leaseWallTime(now.Add(24*time.Hour)) {
		return &ProcessingError{Code: "invalid_time"}
	}
	r.deadline, _ = r.ctx.Deadline()
	if limit := now.Add(120 * time.Second); limit.Before(r.deadline) {
		r.deadline = limit
	}
	work, cancel := context.WithDeadline(r.ctx, r.deadline.Add(-discoveryReserve))
	defer cancel()
	r.ctx = work
	r.storage = &discoveryStorage{objects: r.owner.objects, reserve: r.reserve}
	r.store, _ = ocistorage.NewNoticeStore(r.storage, r.lease.manager.environmentID)
	r.baseline, err = r.owner.selector.Select(r.ctx, r.lease)
	if err != nil {
		return err
	}
	r.final, err = r.owner.discovery.Run(r.ctx, r.lease)
	if err != nil {
		return err
	}
	if !r.baseline.Covers(initial) || !r.baseline.Covers(r.final) {
		return &ProcessingError{Code: "baseline_mismatch"}
	}
	worklist, err := processingWork(initial, r.final)
	if err != nil {
		return err
	}
	for _, work := range worklist {
		if err = r.process(work); err != nil {
			return err
		}
	}
	if err = r.guard(); err != nil {
		return err
	}
	r.result.snapshot.Completed = true
	return nil
}

func (r *processingRun) reserve() error { return currentReserve(r.ctx, r.lease) }
func (r *processingRun) guard() error   { return r.owner.discovery.controls().check(r.ctx, r.lease) }
func (r *processingRun) before() error {
	if r.started {
		if err := r.owner.wait(r.ctx, 100*time.Millisecond); err != nil {
			return processingFailure(r.ctx, &ProcessingError{Code: "wait_failed"})
		}
	}
	r.started = true
	return r.reserve()
}
