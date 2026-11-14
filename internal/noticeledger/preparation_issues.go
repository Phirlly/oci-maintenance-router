package noticeledger

import "slices"

type PreparationObservation struct {
	RevisionDigest    string `json:"revisionDigest"`
	ResourceIndex     int    `json:"resourceIndex"`
	Code              string `json:"code"`
	FirstSeenAt       string `json:"firstSeenAt"`
	LastSeenAt        string `json:"lastSeenAt"`
	ContextDigest     string `json:"contextDigest"`
	RoutingGeneration string `json:"routingGeneration"`
	DestinationID     string `json:"destinationId"`
}
type PreparationIssueSnapshot struct {
	SchemaVersion int                      `json:"schemaVersion"`
	EnvironmentID string                   `json:"environmentId"`
	NoticeDigest  string                   `json:"noticeDigest"`
	Observations  []PreparationObservation `json:"observations"`
}
type PreparationIssues struct {
	data, parent *PreparationIssueSnapshot
	proposal     bool
}
type PreparationFailure struct {
	notice       Record
	observations []PreparationObservation
}

// Notice is the immutable saved context used to derive this evidence. Neither
// failure evidence nor a saved issue grants permission to prepare or send.
func (f PreparationFailure) Notice() Record { return f.notice }
func (p PreparationIssues) Snapshot() PreparationIssueSnapshot {
	if p.data == nil {
		return PreparationIssueSnapshot{}
	}
	s := *p.data
	s.Observations = slices.Clone(s.Observations)
	return s
}

// Follows binds an immediate in-memory proposal, including the first transition
// from an absent record. The store separately binds confirmed absence and identity.
func (p PreparationIssues) Follows(previous PreparationIssues) bool {
	return p.data != nil && p.proposal && p.parent == previous.data
}
