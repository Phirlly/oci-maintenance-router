package delivery

import "time"

// BeginManual proposes one manual attempt; it cannot consume an operator request
// or authorize dispatch. The enclosing action must be consumed atomically.
func (r Record) BeginManual(id string, c AttemptContext) (Record, AttemptRef, error) {
	if r.data == nil || r.data.SchemaVersion != 2 {
		return Record{}, AttemptRef{}, &Error{"attempt", "migration_required"}
	}
	return r.begin(id, c, ManualAttempt)
}

func finishManual(b savedBatch, status int, now, retryAt time.Time) (savedBatch, error) {
	if !retryAt.IsZero() {
		return savedBatch{}, &Error{"retryAt", "unexpected"}
	}
	b.UpdatedAt = timeText(now)
	if status == 202 {
		b.State, b.AcceptedAt = APIAccepted, timeText(now)
		return b, nil
	}
	b.State, b.Reason = ReviewRequired, ReasonPermanent
	if status == 0 {
		b.Reason, b.Ambiguous = ReasonNetwork, true
	} else if status == 429 || status >= 500 {
		b.Reason = ReasonTransient
	}
	return b, nil
}
