package reconciliation

import (
	"context"
	"time"
)

type storedReport struct {
	state  reportState
	etag   string
	absent bool
}

type reportingRun struct {
	owner    *IssueReporter
	ctx      context.Context
	lease    *Lease
	deadline time.Time
	observed time.Time
	token    string
	saved    storedReport
	result   ReportingResult
}

func (d *IssueReporter) run(ctx context.Context, lease *Lease, snapshot *ProcessingSnapshot) (ReportingResult, error) {
	if d == nil || d.objects == nil || (d.proof == nil && d.setupProof == nil) || d.read == nil || d.token == nil || !lease.usable() || leaseRequest(ctx) != nil || lease.manager.environmentID != d.environment {
		return ReportingResult{}, &ReportingError{"invalid_request"}
	}
	deadline, _ := ctx.Deadline()
	if lease.deadline.Before(deadline) {
		deadline = lease.deadline
	}
	work, cancel := context.WithDeadline(ctx, deadline.Add(-discoveryReserve))
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	r := &reportingRun{owner: d, ctx: work, lease: lease, deadline: deadline, token: d.token()}
	if !leaseDigest.MatchString(r.token) || r.token == lease.record.OwnerToken {
		return r.result, &ReportingError{"invalid_request"}
	}
	err := r.execute(snapshot)
	if r.saved.state.Environment != "" {
		r.result.Known = true
		r.result.Issues = len(r.saved.state.Issues)
		r.result.Pending = r.saved.state.Pending != nil
		r.result.Remaining = len(r.saved.state.Priority) + len(r.saved.state.Cursor.Names) - r.saved.state.Cursor.Offset
	}
	return r.result, reportFailure(r.ctx, err)
}

func (r *reportingRun) execute(snapshot *ProcessingSnapshot) error {
	if err := r.guard(); err != nil {
		return err
	}
	saved, err := r.load()
	if err != nil {
		return err
	}
	r.saved = saved
	state := cloneReport(saved.state)
	if err = bindReportChannel(&state, r.owner.channel); err != nil {
		return err
	}
	if err = r.save(state, false); err != nil {
		return err
	}
	if snapshot != nil {
		if err = r.refresh(*snapshot); err != nil {
			return err
		}
	}
	state = cloneReport(r.saved.state)
	if err = queueReport(&state); err != nil {
		return err
	}
	if err = r.save(state, false); err != nil {
		return err
	}
	return r.publish()
}

func (r *reportingRun) guard() error {
	if err := r.owner.controls.check(r.ctx, r.lease); err != nil {
		return reportFailure(r.ctx, err)
	}
	return nil
}

func reportFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &ReportingError{"deadline_exceeded"}
		}
		return &ReportingError{"canceled"}
	}
	if e, ok := err.(*ReportingError); ok && e != nil {
		return e
	}
	if e, ok := err.(*DiscoveryError); ok && e != nil {
		return &ReportingError{e.Code}
	}
	return &ReportingError{"reporting_failed"}
}
