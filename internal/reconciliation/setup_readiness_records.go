package reconciliation

import "context"

func (r *setupReadinessRun) diagnostic(head setupCurrent, confirmation SetupConfirmation, release string) error {
	d, _, err := r.owner.input.diagnosticRecord(r.ctx, r.store.objects, head, release)
	if err != nil {
		return err
	}
	body, _ := d.Encode()
	s := d.Snapshot()
	if s.State != DiagnosticAccepted || diagnosticDigest(string(body)) != confirmation.data.DiagnosticSHA256 {
		return &SetupError{Code: "confirmation_mismatch"}
	}
	if confirmation.confirmedAt().Before(s.AcceptedAt) {
		return &SetupError{Code: "invalid_time"}
	}
	return nil
}

func (r *setupReadinessRun) operator(head setupCurrent, confirmation SetupConfirmation) (OperatorChallenge, error) {
	o, _, err := r.owner.input.operatorRecord(r.ctx, r.store.objects, head)
	if err != nil {
		return OperatorChallenge{}, err
	}
	body, _ := o.Encode()
	s := o.Snapshot()
	if s.State != OperatorAccepted || operatorDigest(string(body)) != confirmation.data.OperatorSHA256 {
		return OperatorChallenge{}, &SetupError{Code: "confirmation_mismatch"}
	}
	if confirmation.confirmedAt().Before(s.AcceptedAt) {
		return OperatorChallenge{}, &SetupError{Code: "invalid_time"}
	}
	if err = currentReserve(r.ctx, r.lease); err != nil {
		return OperatorChallenge{}, err
	}
	ctx, cancel := context.WithTimeout(r.ctx, discoveryReserve)
	channel, err := r.owner.read(ctx)
	late := ctx.Err()
	cancel()
	if err != nil || late != nil {
		return OperatorChallenge{}, operatorChannelReadError(r.ctx, err)
	}
	if channel != r.owner.input.desired {
		return OperatorChallenge{}, &SetupError{Code: "setup_stale"}
	}
	return o, nil
}
