package reconciliation

import "slices"

// UnavailableNotice records only a controlled failed saved-only HTTP read.
// It does not mean deletion or permission to deliver saved content.
type UnavailableNotice struct {
	NoticeID   string
	HTTPStatus int
}

// SourceAvailability is immutable invocation evidence. Successful observations
// live in Collection.Current/Observations. No outcome here is durable yet.
type SourceAvailability struct {
	environmentID string
	unavailable   []UnavailableNotice
}

func (s SourceAvailability) EnvironmentID() string            { return s.environmentID }
func (s SourceAvailability) Unavailable() []UnavailableNotice { return slices.Clone(s.unavailable) }
