package delivery

import (
	"slices"
	"strings"
)

func NewRecord(preparations ...Preparation) (Record, error) {
	if len(preparations) == 0 || len(preparations) > 256 {
		return Record{}, &Error{"preparations", "invalid_count"}
	}
	s := preparations[0].Draft.snapshot
	r := Record{data: &recordData{SchemaVersion: 1, EnvironmentID: s.EnvironmentID, NoticeID: s.Revision.NoticeID}}
	return r.Prepare(preparations...)
}

// Prepare keeps an existing batch verbatim. Only previously unassigned members
// of a revision can enter a different batch, regardless of its current state.
func (r Record) Prepare(preparations ...Preparation) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"record", "invalid"}
	}
	if len(preparations) > 256 {
		return Record{}, &Error{"preparations", "too_large"}
	}
	d := r.copy()
	seen := map[string]bool{}
	for _, b := range d.Batches {
		seen[b.Message.BatchID] = true
	}
	for _, p := range preparations {
		s := p.Draft.snapshot
		if s.BatchID == "" || p.SecretVersion <= 0 {
			return Record{}, &Error{"preparation", "invalid"}
		}
		if s.EnvironmentID != d.EnvironmentID || s.Revision.NoticeID != d.NoticeID {
			return Record{}, &Error{"preparation", "identity_mismatch"}
		}
		if !seen[s.BatchID] {
			d.Batches = append(d.Batches, savedBatch{Message: saveMessage(s), SecretVersion: p.SecretVersion, State: Prepared})
			seen[s.BatchID] = true
		}
	}
	slices.SortFunc(d.Batches, func(a, b savedBatch) int { return strings.Compare(a.Message.BatchID, b.Message.BatchID) })
	if err := validateAssignments(d.Batches); err != nil {
		return Record{}, err
	}
	if err := recordCapacity(&d); err != nil {
		return Record{}, err
	}
	return Record{data: &d}, nil
}
