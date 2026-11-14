package reconciliation

import "context"

// Verify checks pinned attestation against actual saved acceptance and freshly
// observed channel metadata. It never initializes, writes, expires or publishes.
// Later activation must still check all prerequisites under current controls.
func (v *OperatorReceiptVerifier) Verify(ctx context.Context, lease *Lease) (OperatorReceipt, error) {
	if v == nil || !v.store.usable() || !validOperatorIntent(v.input.intent) || v.read == nil || !lease.usable() {
		return OperatorReceipt{}, &OperatorRunError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return OperatorReceipt{}, operatorRunError(ctx, "invalid_request", err)
	}
	if lease.manager.environmentID != v.input.intent.EnvironmentID {
		return OperatorReceipt{}, &OperatorRunError{Code: "environment_mismatch"}
	}
	ctx, cancel := context.WithDeadline(ctx, lease.deadline)
	defer cancel()
	stop := context.AfterFunc(lease.origin, cancel)
	defer stop()
	fail := func(err error) (OperatorReceipt, error) {
		return OperatorReceipt{}, operatorRunError(ctx, "verification_failed", err)
	}
	if err := v.input.current.check(ctx, lease); err != nil {
		return fail(err)
	}
	saved, err := v.store.Load(ctx, v.input.intent.RequestID)
	if err != nil {
		return fail(err)
	}
	r := saved.Record()
	if !r.Matches(v.input.intent) {
		return fail(&OperatorChallengeError{Code: "identity_conflict"})
	}
	s := r.Snapshot()
	if s.State != OperatorAccepted {
		return fail(&OperatorChallengeError{Code: "not_accepted"})
	}
	if v.input.receiptCode != s.ConfirmationCode {
		return fail(&OperatorChallengeError{Code: "receipt_mismatch"})
	}
	if err := currentReserve(ctx, lease); err != nil {
		return fail(err)
	}
	read, closeRead := context.WithTimeout(ctx, discoveryReserve)
	channel, err := v.read(read)
	late := read.Err()
	closeRead()
	if err != nil || late != nil {
		return OperatorReceipt{}, operatorChannelReadError(ctx, err)
	}
	if channel != v.input.desired {
		return fail(&OperatorChallengeError{Code: "channel_changed"})
	}
	if err := v.input.current.check(ctx, lease); err != nil {
		return fail(err)
	}
	now := lease.manager.now()
	if !leaseWallTime(now) || now.Before(s.AcceptedAt) {
		return fail(&OperatorChallengeError{Code: "invalid_time"})
	}
	return OperatorReceipt{r, v.input.current.configHash}, nil
}
