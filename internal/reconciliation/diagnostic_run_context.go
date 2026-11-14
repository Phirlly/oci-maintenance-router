package reconciliation

import (
	"context"
	"time"
)

type diagnosticRun struct {
	owner    *DiagnosticCoordinator
	ctx      context.Context
	lease    *Lease
	deadline time.Time
	token    string
	objects  *diagnosticRunStorage
	store    *DiagnosticStore
	result   DiagnosticResult
}

func (d *DiagnosticCoordinator) start(ctx context.Context, lease *Lease) (*diagnosticRun, context.CancelFunc, error) {
	if d == nil || d.objects == nil || d.current == nil || d.send == nil || d.token == nil || !validDiagnosticIntent(d.intent) || !lease.usable() {
		return nil, nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return nil, nil, diagnosticRunError(ctx, "invalid_request", err)
	}
	if lease.manager.environmentID != d.intent.EnvironmentID {
		return nil, nil, &DiagnosticRunError{Code: "environment_mismatch"}
	}
	token := d.token()
	if !leaseDigest.MatchString(token) || token == lease.record.OwnerToken {
		return nil, nil, &DiagnosticRunError{Code: "invalid_request"}
	}
	deadline, _ := ctx.Deadline()
	if lease.deadline.Before(deadline) {
		deadline = lease.deadline
	}
	work, cancel := context.WithDeadline(ctx, deadline.Add(-discoveryReserve))
	stop := context.AfterFunc(lease.origin, cancel)
	r := &diagnosticRun{owner: d, ctx: work, lease: lease, deadline: deadline, token: token}
	r.objects = &diagnosticRunStorage{objects: d.objects, reserve: r.reserve}
	r.store, _ = NewDiagnosticStore(r.objects, d.intent.EnvironmentID, d.intent.InstallationID)
	return r, func() { stop(); cancel() }, nil
}
func (r *diagnosticRun) reserve() error { return currentReserve(r.ctx, r.lease) }
func (r *diagnosticRun) guard() error   { return r.owner.controls.check(r.ctx, r.lease) }
func (r *diagnosticRun) fail(err error) (DiagnosticResult, error) {
	return r.result, diagnosticRunError(r.ctx, "run_failed", err)
}
