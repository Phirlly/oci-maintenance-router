package reconciliation

import (
	"context"
	"time"
)

func (l *Lease) remaining(ctx context.Context, required time.Duration, work bool) error {
	if work && l.origin.Err() != nil {
		return leaseError(l.origin, "canceled", nil)
	}
	if ctx.Err() != nil {
		return leaseError(ctx, "canceled", nil)
	}
	now := l.manager.now()
	if err := l.record.clock(now); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if work && l.deadline.Before(deadline) {
		deadline = l.deadline
	}
	_, expiry := l.record.times()
	if expiry.Before(deadline) {
		deadline = expiry
	}
	if !now.Before(deadline) {
		return &LeaseError{Code: "deadline_exceeded"}
	}
	if required > deadline.Sub(now) {
		return &LeaseError{Code: "insufficient_time"}
	}
	return nil
}

// Check verifies the exact current claim and remaining budget. It never renews
// authority, and cannot fence an external request after this check completes.
func (l *Lease) Check(ctx context.Context, required time.Duration) error {
	if !l.usable() || required <= 0 {
		return &LeaseError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return err
	}
	if err := l.remaining(ctx, required, true); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, l.deadline)
	defer cancel()
	stop := context.AfterFunc(l.origin, cancel)
	defer stop()
	record, tag, err := l.manager.load(ctx)
	if check := l.remaining(ctx, required, true); check != nil {
		return check
	}
	if err != nil {
		return err
	}
	if record != l.record || tag != l.etag {
		return &LeaseError{Code: "ownership_lost"}
	}
	return nil
}

// Release supports explicit bounded cleanup after work cancellation. It never
// deletes state or overwrites a replacement owner; it does not report scan success.
func (l *Lease) Release(ctx context.Context) error {
	if !l.usable() {
		return &LeaseError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return err
	}
	if err := l.remaining(ctx, 0, false); err != nil {
		return err
	}
	_, expiry := l.record.times()
	ctx, cancel := context.WithDeadline(ctx, expiry)
	defer cancel()
	released := l.record
	released.ExpiresAt = released.AcquiredAt
	_, err := l.manager.write(ctx, released, l.etag)
	if leaseConflict(err) {
		_, err = l.manager.confirm(ctx, released)
	}
	if check := l.remaining(ctx, 0, false); check != nil {
		return check
	}
	return err
}
