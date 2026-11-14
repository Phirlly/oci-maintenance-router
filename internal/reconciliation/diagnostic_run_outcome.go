package reconciliation

import (
	"context"
	"time"
)

func (r *diagnosticRun) dispatch(saved StoredDiagnostic, credential diagnosticCredential) (DiagnosticResult, error) {
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	payload := saved.record.Snapshot().PayloadWithoutRoutingKey
	key := credential.Bytes()
	defer clear(key) // Best-effort transient-copy cleanup, not secure erasure.
	if err := r.reserve(); err != nil {
		return r.fail(err)
	}
	r.result.dispatched = true
	response, _ := r.owner.send(r.ctx, payload, key)
	// A valid status is observed evidence even when cancellation follows headers.
	// Errors and nonstandard statuses remain unknown; diagnostics never retry.
	if response.Status >= 100 && response.Status <= 599 {
		r.result.status = response.Status
	}
	return r.finish(saved)
}
func (r *diagnosticRun) finish(saved StoredDiagnostic) (DiagnosticResult, error) {
	at := r.lease.manager.now()
	next, err := saved.record.Finish(DiagnosticClaimRef{r.lease.record.OwnerToken, r.token}, r.result.status, at)
	if err != nil {
		return r.result, diagnosticRunError(nil, "outcome_invalid", err)
	}
	// Only this already-issued operation's exact outcome may outlive work
	// cancellation. The original deadline is restored, never extended.
	receipt, cancel := context.WithDeadline(context.WithoutCancel(r.ctx), r.deadline)
	defer cancel()
	r.objects.reserve = func() error { return r.receiptReserve(receipt, at) }
	if err := r.objects.reserve(); err != nil {
		return r.result, err
	}
	got, err := r.store.Save(receipt, saved, next)
	if err != nil {
		return r.result, diagnosticRunError(receipt, "outcome_unconfirmed", err)
	}
	r.result.record = got.record
	return r.result, nil
}
func (r *diagnosticRun) receiptReserve(ctx context.Context, observed time.Time) error {
	if ctx.Err() != nil {
		return diagnosticRunError(ctx, "outcome_unconfirmed", nil)
	}
	now := r.lease.manager.now()
	if !leaseWallTime(now) || now.Before(observed) {
		return &DiagnosticRunError{Code: "clock_invalid"}
	}
	if !now.Before(r.deadline) {
		return &DiagnosticRunError{Code: "deadline_exceeded"}
	}
	return nil
}
