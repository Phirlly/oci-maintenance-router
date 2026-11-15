package reconciliation

import (
	"context"
	"time"
)

func (r *operatorRun) dispatch(saved StoredOperatorChallenge) (OperatorResult, error) {
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	channel, err := r.channel()
	if err != nil {
		return r.result, err
	}
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	payload := saved.record.Snapshot().Payload
	if err := r.reserve(); err != nil {
		return r.fail(err)
	}
	r.result.dispatched = true
	response, failure := channel.publish(r.ctx, payload)
	if response.Status >= 100 && response.Status <= 599 {
		r.result.status = response.Status
	}
	// Preserve status even after an incomplete response; a valid message ID is
	// evidence only when the bounded adapter completed a valid200/202 response.
	if (response.Status == 200 || response.Status == 202) && failure == nil && operatorOpaqueText(response.MessageID, 256) {
		r.result.messageID = response.MessageID
	}
	return r.finish(saved)
}
func (r *operatorRun) finish(saved StoredOperatorChallenge) (OperatorResult, error) {
	at := r.lease.manager.now()
	next, err := saved.record.Finish(OperatorClaimRef{r.lease.record.OwnerToken, r.token}, r.result.status, r.result.messageID, at)
	if err != nil {
		return r.result, operatorRunError(nil, "outcome_invalid", err)
	}
	// Only this already-issued outcome may outlive work cancellation. Restore
	// the original deadline, never a fresh send/control/claim lifetime.
	receipt, cancel := context.WithDeadline(context.WithoutCancel(r.ctx), r.deadline)
	defer cancel()
	r.objects.reserve = func() error { return r.receiptReserve(receipt, at) }
	if err := r.objects.reserve(); err != nil {
		return r.result, err
	}
	got, err := r.store.Save(receipt, saved, next)
	if err != nil {
		return r.result, operatorRunError(receipt, "outcome_unconfirmed", err)
	}
	r.result.record = got.record
	return r.result, nil
}
func (r *operatorRun) receiptReserve(ctx context.Context, observed time.Time) error {
	if ctx.Err() != nil {
		return operatorRunError(ctx, "outcome_unconfirmed", nil)
	}
	now := r.lease.manager.now()
	if !leaseWallTime(now) || now.Before(observed) {
		return &OperatorRunError{Code: "clock_invalid"}
	}
	if !now.Before(r.deadline) {
		return &OperatorRunError{Code: "deadline_exceeded"}
	}
	return nil
}
