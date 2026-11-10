package reconciliation

import (
	"oci-maintenance-router/internal/notices"
	"slices"
)

// CandidateRevision is one observed revision and exact chain link. Empty ChainID
// means no edge; it is not a wildcard or a claim that the source is available.
type CandidateRevision struct {
	Revision notices.RevisionKey
	ChainID  string
}

// CandidateComponent is an owned snapshot of unique relevant membership.
// Its fingerprint identifies membership, not authorization or source freshness.
type CandidateComponent struct {
	Fingerprint string
	Members     []CandidateRevision
}

// CandidateView is immutable observed evidence. It includes saved-only history
// but does not establish complete discovery, availability or permission to send.
type CandidateView struct {
	environmentID string
	current       []CandidateMatch
	components    []CandidateComponent
}

func (v CandidateView) EnvironmentID() string     { return v.environmentID }
func (v CandidateView) Current() []CandidateMatch { return slices.Clone(v.current) }
func (v CandidateView) Components() []CandidateComponent {
	result := slices.Clone(v.components)
	for i := range result {
		result[i].Members = slices.Clone(result[i].Members)
	}
	return result
}
