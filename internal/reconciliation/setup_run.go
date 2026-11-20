package reconciliation

import "context"

type setupRun struct {
	owner  *SetupCoordinator
	ctx    context.Context
	lease  *Lease
	store  *setupCurrentStore
	result SetupResult
}

func (r *setupRun) guard() error {
	if err := r.owner.input.current.check(r.ctx, r.lease); err != nil {
		e := discoveryFailureFor(r.ctx, "setup_guard", err)
		return setupFailure(r.ctx, e.Code, e.HTTPStatus, false)
	}
	return nil
}

func (r *setupRun) send() error {
	c := r.owner
	if c.diagnostic == nil || c.operator == nil {
		return &SetupError{Code: "invalid_request"}
	}
	d, o := c.input.requested(c.release)
	old, missing, err := r.store.load(r.ctx)
	if err != nil && !missing {
		return err
	}
	if missing {
		proposal, failure := newSetupCurrent(c.input.config.Setup.Attempt, d, o)
		if failure != nil {
			return failure
		}
		_, err = r.store.create(r.ctx, proposal)
	} else {
		next, failure := old.record.selectAttempt(c.input.config.Setup.Attempt, d, o)
		if failure != nil {
			return failure
		}
		if next.data != old.record.data {
			_, err = r.store.save(r.ctx, old, next)
		}
	}
	if err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	r.result.Diagnostic, err = c.diagnostic.Run(r.ctx, r.lease)
	if err != nil {
		return err
	}
	if r.result.Diagnostic.Record().Snapshot().State != DiagnosticAccepted {
		return &SetupError{Code: "diagnostic_not_accepted"}
	}
	if err = r.guard(); err != nil {
		return err
	}
	r.result.Operator, err = c.operator.Run(r.ctx, r.lease)
	if err != nil {
		return err
	}
	if r.result.Operator.Record().Snapshot().State != OperatorAccepted {
		return &SetupError{Code: "operator_not_accepted"}
	}
	return nil
}
