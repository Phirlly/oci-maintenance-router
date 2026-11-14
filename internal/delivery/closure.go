package delivery

import "time"

// Close proposes terminal closure without claiming acceptance or operator intent.
func (r Record) Close(id string, at time.Time) (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 2 {
		return Record{}, &Error{"closure", "migration_required"}
	}
	i, err := r.index(id)
	if err != nil {
		return Record{}, err
	}
	b := r.data.Batches[i]
	if b.State != Prepared && b.State != RetryWait && b.State != ReviewRequired {
		return Record{}, &Error{"state", "invalid_transition"}
	}
	now, err := operationTime(at, b.UpdatedAt)
	if err != nil {
		return Record{}, err
	}
	b.State, b.Reason, b.UpdatedAt, b.NextAttemptAt = Closed, ReasonOperator, timeText(now), nil
	d := r.copy()
	d.Batches[i] = b
	return Record{data: &d}, nil
}
