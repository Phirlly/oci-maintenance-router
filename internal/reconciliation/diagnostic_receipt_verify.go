package reconciliation

import "context"

// Verify checks the operator's pinned attestation against the actual saved test.
// It never reads credentials, sends, initializes or writes state. Later activation
// must still check its other prerequisites under the same current configuration.
func (v *DiagnosticReceiptVerifier) Verify(ctx context.Context, lease *Lease) (DiagnosticReceipt, error) {
	if v == nil || !v.store.usable() || !validDiagnosticIntent(v.input.intent) || !lease.usable() {
		return DiagnosticReceipt{}, &DiagnosticRunError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return DiagnosticReceipt{}, diagnosticRunError(ctx, "invalid_request", err)
	}
	if lease.manager.environmentID != v.input.intent.EnvironmentID {
		return DiagnosticReceipt{}, &DiagnosticRunError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	fail := func(err error) (DiagnosticReceipt, error) {
		return DiagnosticReceipt{}, diagnosticRunError(ctx, "verification_failed", err)
	}
	if err := v.input.current.check(ctx, lease); err != nil {
		return fail(err)
	}
	// The guard just checked the per-operation reserve. Load adds its own10s cap
	// and validates canonical content, exact identity and the observed ETag.
	saved, err := v.store.Load(ctx, v.input.intent.RequestID)
	if err != nil {
		return fail(err)
	}
	r := saved.Record()
	if !r.Matches(v.input.intent) {
		return fail(&DiagnosticError{Code: "identity_conflict"})
	}
	s := r.Snapshot()
	if s.State != DiagnosticAccepted {
		return fail(&DiagnosticError{Code: "not_accepted"})
	}
	if v.input.receiptCode != s.ConfirmationCode {
		return fail(&DiagnosticError{Code: "receipt_mismatch"})
	}
	if err := v.input.current.check(ctx, lease); err != nil {
		return fail(err)
	}
	now := lease.manager.now()
	if !leaseWallTime(now) || now.Before(s.AcceptedAt) {
		return fail(&DiagnosticError{Code: "invalid_time"})
	}
	return DiagnosticReceipt{r, v.input.current.configHash}, nil
}
