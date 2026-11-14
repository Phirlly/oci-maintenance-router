package reconciliation

import (
	"context"
	"time"
)

type operatorRun struct {
	owner    *OperatorCoordinator
	ctx      context.Context
	lease    *Lease
	deadline time.Time
	token    string
	objects  *operatorRunStorage
	store    *OperatorChallengeStore
	result   OperatorResult
}

func (d *OperatorCoordinator) start(ctx context.Context, lease *Lease) (*operatorRun, context.CancelFunc, error) {
	if d == nil || d.objects == nil || d.read == nil || d.token == nil || !validOperatorIntent(d.intent) || !lease.usable() {
		return nil, nil, &OperatorRunError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return nil, nil, operatorRunError(ctx, "invalid_request", err)
	}
	if lease.manager.environmentID != d.intent.EnvironmentID {
		return nil, nil, &OperatorRunError{Code: "environment_mismatch"}
	}
	token := d.token()
	if !leaseDigest.MatchString(token) || token == lease.record.OwnerToken {
		return nil, nil, &OperatorRunError{Code: "invalid_request"}
	}
	deadline, _ := ctx.Deadline()
	if lease.deadline.Before(deadline) {
		deadline = lease.deadline
	}
	work, cancel := context.WithDeadline(ctx, deadline.Add(-discoveryReserve))
	stop := context.AfterFunc(lease.origin, cancel)
	r := &operatorRun{owner: d, ctx: work, lease: lease, deadline: deadline, token: token}
	r.objects = &operatorRunStorage{objects: d.objects, reserve: r.reserve}
	r.store, _ = NewOperatorChallengeStore(r.objects, d.intent.EnvironmentID, d.intent.InstallationID)
	return r, func() { stop(); cancel() }, nil
}
func (r *operatorRun) reserve() error { return currentReserve(r.ctx, r.lease) }
func (r *operatorRun) guard() error   { return r.owner.controls.check(r.ctx, r.lease) }
func (r *operatorRun) fail(err error) (OperatorResult, error) {
	return r.result, operatorRunError(r.ctx, "run_failed", err)
}
