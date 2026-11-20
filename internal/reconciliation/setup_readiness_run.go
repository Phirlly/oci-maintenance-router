package reconciliation

import "context"

type setupReadinessRun struct {
	owner *setupReadiness
	ctx   context.Context
	lease *Lease
	store *setupCurrentStore
}

func (r *setupReadinessRun) guard() error {
	if err := r.owner.input.current.check(r.ctx, r.lease); err != nil {
		e := discoveryFailureFor(r.ctx, "setup_guard", err)
		return setupFailure(r.ctx, e.Code, e.HTTPStatus, false)
	}
	return nil
}

func (r *setupReadinessRun) verify(release string) (OperatorReceipt, error) {
	if err := r.guard(); err != nil {
		return OperatorReceipt{}, err
	}
	head, _, err := r.store.load(r.ctx)
	if err != nil {
		return OperatorReceipt{}, err
	}
	confirmation, err := r.confirmation(head.record)
	if err != nil {
		return OperatorReceipt{}, err
	}
	if release != "" {
		if err = r.diagnostic(head.record, confirmation, release); err != nil {
			return OperatorReceipt{}, err
		}
	}
	operator, err := r.operator(head.record, confirmation)
	if err != nil {
		return OperatorReceipt{}, err
	}
	current, _, err := r.store.load(r.ctx)
	if err != nil {
		return OperatorReceipt{}, err
	}
	if current.etag != head.etag || !sameSetupCurrent(current.record, head.record) {
		return OperatorReceipt{}, &SetupError{Code: "selection_changed"}
	}
	if err = r.guard(); err != nil {
		return OperatorReceipt{}, err
	}
	now := r.lease.manager.now()
	if !leaseWallTime(now) || now.Before(confirmation.confirmedAt()) {
		return OperatorReceipt{}, &SetupError{Code: "invalid_time"}
	}
	return OperatorReceipt{operator, r.owner.input.current.configHash}, nil
}

func (r *setupReadinessRun) confirmation(head setupCurrent) (SetupConfirmation, error) {
	ref := head.data.Confirmation
	if ref == nil {
		return SetupConfirmation{}, &SetupError{Code: "confirmation_required"}
	}
	s, _ := NewSetupConfirmationStore(r.store.objects, r.owner.input.environment, r.owner.input.config.InstallationID)
	ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
	defer cancel()
	c, _, err := s.load(ctx, ref.ID)
	if err != nil {
		return SetupConfirmation{}, err
	}
	b, _ := c.Encode()
	if diagnosticDigest(string(b)) != ref.SHA256 || c.data.DiagnosticRequestID != head.data.DiagnosticRequestID || c.data.OperatorRequestID != head.data.OperatorRequestID {
		return SetupConfirmation{}, &SetupError{Code: "confirmation_mismatch"}
	}
	return c, nil
}
