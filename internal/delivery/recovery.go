package delivery

import "time"

// Recover never guesses whether an expired send reached PagerDuty.
func (r Record) Recover(at time.Time) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"record", "invalid"}
	}
	now, err := operationTime(at, nil)
	if err != nil {
		return Record{}, err
	}
	for _, b := range r.data.Batches {
		if _, err := operationTime(now, b.UpdatedAt); err != nil {
			return Record{}, err
		}
	}
	d := r.copy()
	for i, b := range d.Batches {
		if b.State == Sending && !now.Before(savedTime(b.AttemptExpiresAt)) {
			b.State, b.Reason, b.Ambiguous = RetryWait, ReasonExpired, true
			b.UpdatedAt, b.NextAttemptAt = timeText(now), timeText(now)
		}
		if b.State == RetryWait && !now.Before(savedTime(b.FirstAttemptAt).Add(retryHorizon)) {
			b.State, b.Reason, b.NextAttemptAt = ReviewRequired, ReasonHorizon, nil
			b.UpdatedAt = timeText(now)
		}
		d.Batches[i] = b
	}
	return Record{data: &d}, nil
}

func (r Record) Hold(id string, reason Reason, at time.Time) (Record, error) {
	i, err := r.index(id)
	if err != nil {
		return Record{}, err
	}
	b := r.data.Batches[i]
	now, err := operationTime(at, b.UpdatedAt)
	if err != nil {
		return Record{}, err
	}
	if !holdReason(reason) {
		return Record{}, &Error{"reason", "invalid"}
	}
	if b.State != Prepared && b.State != RetryWait {
		return Record{}, &Error{"state", "invalid_transition"}
	}
	b.State, b.Reason, b.UpdatedAt, b.NextAttemptAt = ReviewRequired, reason, timeText(now), nil
	d := r.copy()
	d.Batches[i] = b
	return Record{data: &d}, nil
}

func holdReason(reason Reason) bool {
	switch reason {
	case ReasonSourceChanged, ReasonSourceUnavailable, ReasonConfigurationChanged, ReasonSecretUnavailable, ReasonOperator:
		return true
	}
	return false
}
