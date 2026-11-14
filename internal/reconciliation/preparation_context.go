package reconciliation

import "context"

type preparationRun struct {
	owner     *BaselinePreparation
	ctx       context.Context
	lease     *Lease
	baselines *BaselineStore
	history   *PreparedHistoryStore
}

func (p *BaselinePreparation) start(ctx context.Context, lease *Lease, initialize bool) (*preparationRun, context.CancelFunc, error) {
	if p == nil || p.discovery == nil || !lease.usable() {
		return nil, nil, &PreparationError{Code: "invalid_request"}
	}
	if p.mode != "PREVIEW" || initialize && !p.initialize {
		return nil, nil, &PreparationError{Code: "intent_required"}
	}
	if err := leaseRequest(ctx); err != nil {
		return nil, nil, preparationFailure(ctx, "invalid_request", err)
	}
	environment := p.discovery.collector.environmentID
	if lease.manager.environmentID != environment {
		return nil, nil, &PreparationError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	stop := context.AfterFunc(lease.origin, cancel)
	bounded, finish, err := discoveryContext(ctx)
	if err != nil {
		stop()
		cancel()
		return nil, nil, preparationFailure(ctx, "invalid_request", err)
	}
	r := &preparationRun{owner: p, ctx: bounded, lease: lease}
	objects := discoveryStorage{objects: p.objects, reserve: r.reserve}
	// Both identities/backends were validated at construction. Handles and budgets
	// are owned by this operation, not shared between concurrent calls.
	r.baselines, _ = NewBaselineStore(objects, environment)
	r.history, _ = NewPreparedHistoryStore(objects, environment, p.installation)
	return r, func() { finish(); stop(); cancel() }, nil
}
func (r *preparationRun) reserve() error { return currentReserve(r.ctx, r.lease) }
func (r *preparationRun) guard() error   { return r.owner.discovery.controls().check(r.ctx, r.lease) }

// Only trusted store errors reach this classifier; raw backend errors do not.
func preparationMissing(ctx context.Context, err error) bool {
	e, ok := err.(*BaselineStorageError)
	return ctx.Err() == nil && ok && e != nil && e.Code == "read_failed" && e.HTTPStatus == 404
}
