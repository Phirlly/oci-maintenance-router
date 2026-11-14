package reconciliation

import "context"

type liveSelectionRun struct {
	owner     *LiveBaselineSelector
	ctx       context.Context
	lease     *Lease
	baselines *BaselineStore
	history   *PreparedHistoryStore
	selected  *BaselineSelectionStore
}

// Select confirms the currently requested cumulative baseline. The result is not
// fresh discovery, a completed processing pass or future permission to send.
// The caller owns lease release and all later processing/dispatch checks.
func (s *LiveBaselineSelector) Select(ctx context.Context, lease *Lease) (Baseline, error) {
	if s == nil || s.diagnostic == nil || s.operator == nil || !lease.usable() {
		return Baseline{}, &LiveSelectionError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return Baseline{}, liveSelectionError(ctx, err)
	}
	env := s.diagnostic.input.intent.EnvironmentID
	if lease.manager.environmentID != env {
		return Baseline{}, &LiveSelectionError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	r := &liveSelectionRun{owner: s, ctx: ctx, lease: lease}
	objects := liveSelectionStorage{s.objects, r.reserve}
	// Constructor validation established the shared environment, installation and
	// backend. Stores and handles belong only to this invocation.
	r.baselines, _ = NewBaselineStore(objects, env)
	r.history, _ = NewPreparedHistoryStore(objects, env, s.diagnostic.input.intent.InstallationID)
	r.selected, _ = NewBaselineSelectionStore(objects, env, s.diagnostic.input.intent.InstallationID)
	b, err := r.run()
	if err == nil {
		err = r.guard()
	}
	if err == nil {
		err = r.checkTime(b)
	}
	if err != nil {
		if lease.origin.Err() != nil {
			ctx = lease.origin
		}
		return Baseline{}, liveSelectionError(ctx, err)
	}
	return b, nil
}

func (r *liveSelectionRun) reserve() error { return currentReserve(r.ctx, r.lease) }
func (r *liveSelectionRun) guard() error {
	return r.owner.diagnostic.input.current.check(r.ctx, r.lease)
}
func (r *liveSelectionRun) checkTime(b Baseline) error {
	now := r.lease.manager.now()
	if !leaseWallTime(now) || now.Before(b.CompletedAt()) {
		return &LiveSelectionError{Code: "invalid_time"}
	}
	return nil
}

func (r *liveSelectionRun) run() (Baseline, error) {
	if _, err := r.owner.diagnostic.Verify(r.ctx, r.lease); err != nil {
		return Baseline{}, err
	}
	if _, err := r.owner.operator.Verify(r.ctx, r.lease); err != nil {
		return Baseline{}, err
	}
	if err := r.guard(); err != nil {
		return Baseline{}, err
	}
	target, err := r.target()
	if err != nil {
		return Baseline{}, err
	}
	if err = r.selectTarget(target); err != nil {
		return Baseline{}, err
	}
	return target, nil
}
