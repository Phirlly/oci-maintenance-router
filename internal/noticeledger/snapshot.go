package noticeledger

import "slices"

func copyText(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}
func copySource(s Source) Source {
	s.ChainID, s.Reference = copyText(s.ChainID), copyText(s.Reference)
	s.TimeCreated, s.TimeUpdated = copyText(s.TimeCreated), copyText(s.TimeUpdated)
	for i, t := range s.Times {
		s.Times[i] = SemanticTime{copyText(t.Type), copyText(t.Title), copyText(t.Value)}
	}
	return s
}
func (d *recordData) copy() recordData {
	n := *d
	n.Revisions = slices.Clone(d.Revisions)
	for i, v := range n.Revisions {
		v.Source = copySource(v.Source)
		v.Resources = slices.Clone(v.Resources)
		n.Revisions[i] = v
	}
	return n
}
func (r Record) Snapshot() Snapshot {
	if r.data == nil {
		return Snapshot{}
	}
	d := r.data.copy()
	return Snapshot{d.EnvironmentID, d.NoticeID, d.UpdatedAt, d.HoldReason, d.Revisions, d.delivery.Batches()}
}

// Follows identifies exactly one transition from this in-memory record. It is
// used with a stored ETag; it is not a durable identity or permission to send.
func (r Record) Follows(previous Record) bool {
	return r.data != nil && previous.data != nil && r.parent == previous.data
}

func (r Record) Identity() (environmentID, noticeID string) {
	if r.data != nil {
		return r.data.EnvironmentID, r.data.NoticeID
	}
	return "", ""
}
