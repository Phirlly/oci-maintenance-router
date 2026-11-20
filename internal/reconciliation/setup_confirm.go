package reconciliation

func (r *setupRun) confirm() error {
	c := r.owner
	head, _, err := r.store.load(r.ctx)
	if err != nil {
		return err
	}
	if head.record.data.Attempt != c.input.config.Setup.Attempt {
		return &SetupError{Code: "attempt_mismatch"}
	}
	_, d, err := c.input.diagnosticRecord(r.ctx, r.store.objects, head.record, c.release)
	if err != nil {
		return err
	}
	dv, err := newDiagnosticReceiptVerifier(c.input.diagnosticInput(d, c.input.config.Setup.PagerDutyCode), r.store.objects)
	if err != nil {
		return err
	}
	r.result.Confirmation.Diagnostic, err = dv.Verify(r.ctx, r.lease)
	if err != nil {
		return err
	}
	_, o, err := c.input.operatorRecord(r.ctx, r.store.objects, head.record)
	if err != nil {
		return err
	}
	ov, err := newOperatorReceiptVerifier(c.input.operatorInput(o, c.input.config.Setup.OperatorCode), r.store.objects, c.channel)
	if err != nil {
		return err
	}
	ov.read = c.read
	r.result.Confirmation.Operator, err = ov.Verify(r.ctx, r.lease)
	if err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	proposal, err := newSetupConfirmation(r.result.Confirmation.Diagnostic, r.result.Confirmation.Operator, r.lease.manager.now())
	if err != nil {
		return err
	}
	objects := setupConfirmationStorage{c.objects, func() error { return currentReserve(r.ctx, r.lease) }, r.guard}
	confirmations, _ := NewSetupConfirmationStore(objects, c.input.environment, c.input.config.InstallationID)
	confirmed, err := confirmations.Create(r.ctx, proposal)
	if err != nil {
		return err
	}
	next, err := head.record.attach(confirmed)
	if err != nil {
		return err
	}
	if next.data != head.record.data {
		if _, err = r.store.save(r.ctx, head, next); err != nil {
			return err
		}
	}
	current, _, err := r.store.load(r.ctx)
	if err != nil {
		return err
	}
	if !sameSetupCurrent(current.record, next) {
		return &SetupError{Code: "selection_changed"}
	}
	if err = r.guard(); err != nil {
		return err
	}
	now := r.lease.manager.now()
	if !leaseWallTime(now) || now.Before(confirmed.confirmedAt()) {
		return &SetupError{Code: "invalid_time"}
	}
	r.result.Confirmation.Confirmation = confirmed
	return nil
}
