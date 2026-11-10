package reconciliation

import (
	"context"
	"time"
)

// Acquire returns nil,nil only when a validated other owner is still active.
// A coalesced invocation has not completed discovery or processed any notice.
func (m *LeaseManager) Acquire(ctx context.Context) (*Lease, error) {
	if !m.usable() {
		return nil, &LeaseError{Code: "invalid_request"}
	}
	if err := leaseRequest(ctx); err != nil {
		return nil, err
	}
	start := m.now()
	expiry := start.UTC().Truncate(time.Millisecond).Add(leaseDuration)
	if !leaseWallTime(start) || !leaseWallTime(expiry) {
		return nil, &LeaseError{Code: "clock_invalid"}
	}
	owner := m.token()
	if !leaseDigest.MatchString(owner) {
		return nil, &LeaseError{Code: "invalid_request"}
	}
	deadline, _ := ctx.Deadline()
	if cap := start.Add(leaseWorkDuration); cap.Before(deadline) {
		deadline = cap
	}
	original := ctx
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	record := leaseRecord{1, m.environmentID, owner, start.UTC().Truncate(time.Millisecond).Format(leaseTimeFormat), expiry.Format(leaseTimeFormat)}
	candidate := &Lease{m, record, "", original, deadline}
	if err := candidate.remaining(ctx, 0, true); err != nil {
		return nil, err
	}
	tag, err := m.write(ctx, record, "")
	if leaseConflict(err) {
		current, previous, readErr := m.load(ctx)
		if readErr != nil {
			return nil, readErr
		}
		if err := candidate.remaining(ctx, 0, true); err != nil {
			return nil, err
		}
		now := m.now()
		acquired, expires := current.times()
		if now.Before(acquired) {
			return nil, &LeaseError{Code: "clock_invalid"}
		}
		if now.Before(expires) {
			return nil, nil
		}
		tag, err = m.write(ctx, record, previous)
	}
	if err != nil {
		return nil, err
	}
	if err = candidate.remaining(ctx, 0, true); err != nil {
		return nil, err
	}
	candidate.etag = tag
	return candidate, nil
}
