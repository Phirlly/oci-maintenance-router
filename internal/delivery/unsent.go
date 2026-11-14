package delivery

import "time"

// StopUnsent proposes review for an exact attempt known not to have dispatched.
// The caller must establish that fact; this pure transition grants no authority.
func (r Record) StopUnsent(ref AttemptRef, at time.Time) (Record, error) {
	i, err := r.index(ref.BatchID)
	if err != nil {
		return Record{}, err
	}
	b := r.data.Batches[i]
	now, err := operationTime(at, b.UpdatedAt)
	if err != nil {
		return Record{}, err
	}
	if b.State != Sending || b.OwnerToken != ref.OwnerToken || b.AttemptCount != ref.AttemptCount || !now.Before(savedTime(b.AttemptExpiresAt)) {
		return Record{}, &Error{"attempt", "stale"}
	}
	// Generic operator review is already representable by the existing codec.
	// It records neither an invented response nor new ambiguity. Prior uncertain
	// attempts remain uncertain even though this particular call did not dispatch.
	b.State, b.Reason, b.UpdatedAt = ReviewRequired, ReasonOperator, timeText(now)
	d := r.copy()
	d.Batches[i] = b
	return Record{data: &d}, nil
}
