package delivery

import (
	"math"
	"time"
)

const retryHorizon = 24 * time.Hour
const maxAttemptLease = 600 * time.Second

// Begin proposes an attempt. The returned reference is only for matching a
// response; a successful conditional write and authority checks must precede send.
func (r Record) Begin(id string, c AttemptContext) (Record, AttemptRef, error) {
	i, err := r.index(id)
	if err != nil {
		return Record{}, AttemptRef{}, err
	}
	b := r.data.Batches[i]
	now, err := operationTime(c.Now, b.UpdatedAt)
	if err != nil {
		return Record{}, AttemptRef{}, err
	}
	expires, err := operationTime(c.LeaseExpiresAt, nil)
	if err != nil {
		return Record{}, AttemptRef{}, err
	}
	if !recordOwner.MatchString(c.OwnerToken) || c.RoutingGeneration != b.Message.RoutingGeneration {
		return Record{}, AttemptRef{}, &Error{"attempt", "authority_mismatch"}
	}
	if !expires.After(now) || expires.Sub(now) > maxAttemptLease {
		return Record{}, AttemptRef{}, &Error{"attempt", "invalid_expiry"}
	}
	if b.State != Prepared && b.State != RetryWait {
		return Record{}, AttemptRef{}, &Error{"state", "invalid_transition"}
	}
	if b.State == RetryWait && now.Before(savedTime(b.NextAttemptAt)) {
		return Record{}, AttemptRef{}, &Error{"attempt", "not_due"}
	}
	if b.FirstAttemptAt != nil && !now.Before(savedTime(b.FirstAttemptAt).Add(retryHorizon)) {
		return Record{}, AttemptRef{}, &Error{"attempt", "retry_horizon"}
	}
	if b.AttemptCount == math.MaxUint64 {
		return Record{}, AttemptRef{}, &Error{"attempt", "count_exhausted"}
	}
	b.AttemptCount++
	if b.FirstAttemptAt == nil {
		b.FirstAttemptAt = timeText(now)
	}
	b.State, b.OwnerToken, b.Reason = Sending, c.OwnerToken, ""
	b.LastAttemptAt, b.UpdatedAt, b.AttemptExpiresAt = timeText(now), timeText(now), timeText(expires)
	b.NextAttemptAt = nil
	d := r.copy()
	d.Batches[i] = b
	return Record{data: &d}, AttemptRef{id, c.OwnerToken, b.AttemptCount}, nil
}

// Finish records an observed result as a candidate, never durable success.
// Status zero means an unknown network outcome; raw errors/bodies are excluded.
func (r Record) Finish(ref AttemptRef, status int, at, retryAt time.Time) (Record, error) {
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
	if status != 0 && (status < 100 || status > 599) {
		return Record{}, &Error{"response", "invalid_status"}
	}
	retry := status == 0 || status == 429 || status >= 500
	if !retry && !retryAt.IsZero() {
		return Record{}, &Error{"retryAt", "unexpected"}
	}
	b.UpdatedAt = timeText(now)
	if status == 202 {
		b.State, b.AcceptedAt = APIAccepted, timeText(now)
	} else if retry {
		next, err := operationTime(retryAt, b.UpdatedAt)
		if err != nil {
			return Record{}, err
		}
		b.State, b.Reason, b.NextAttemptAt = RetryWait, ReasonTransient, timeText(next)
		if status == 0 {
			b.Reason, b.Ambiguous = ReasonNetwork, true
		}
		if !next.Before(savedTime(b.FirstAttemptAt).Add(retryHorizon)) {
			b.State, b.Reason, b.NextAttemptAt = ReviewRequired, ReasonHorizon, nil
		}
	} else {
		b.State, b.Reason = ReviewRequired, ReasonPermanent
	}
	d := r.copy()
	d.Batches[i] = b
	return Record{data: &d}, nil
}
