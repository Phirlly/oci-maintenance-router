// Package noticeledger owns complete immutable notice-state proposals.
// Neither a proposal nor successful persistence grants permission to send.
package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

type Source struct {
	ChainID     *string         `json:"chainId"`
	Reference   *string         `json:"reference"`
	Times       [2]SemanticTime `json:"times"`
	TimeCreated *string         `json:"timeCreated"`
	TimeUpdated *string         `json:"timeUpdated"`
}
type SemanticTime struct {
	Type  *string `json:"type"`
	Title *string `json:"title"`
	Value *string `json:"value"`
}
type Resource struct {
	Index         int    `json:"index"`
	ResourceID    string `json:"resourceId"`
	Region        string `json:"region"`
	Disposition   string `json:"disposition"`
	Reason        string `json:"reason"`
	DestinationID string `json:"destinationId"`
	BatchID       string `json:"batchId"`
}
type Revision struct {
	Digest            string                  `json:"digest"`
	Source            Source                  `json:"source"`
	FirstSeenAt       string                  `json:"firstSeenAt"`
	LastSeenAt        string                  `json:"lastSeenAt"`
	RoutingGeneration string                  `json:"routingGeneration"`
	Assessment        routing.AssessmentState `json:"assessment"`
	AssessmentReason  routing.Reason          `json:"assessmentReason"`
	Resources         []Resource              `json:"resources"`
}
type Snapshot struct {
	SchemaVersion                      int
	EnvironmentID, NoticeID, UpdatedAt string
	HoldReason                         delivery.Reason
	Revisions                          []Revision
	SourceOutcome                      *SourceOutcome
	Batches                            []delivery.BatchSnapshot
}
type Record struct {
	data   *recordData
	parent *recordData
}
type recordData struct {
	SchemaVersion int             `json:"schemaVersion"`
	EnvironmentID string          `json:"environmentId"`
	NoticeID      string          `json:"noticeId"`
	UpdatedAt     string          `json:"updatedAt"`
	HoldReason    delivery.Reason `json:"holdReason"`
	Revisions     []Revision      `json:"revisions"`
	SourceOutcome *SourceOutcome  `json:"sourceOutcome,omitempty"`
	delivery      delivery.Record
}
type Error struct{ Code string }

func (e *Error) Error() string { return "notice ledger: " + e.Code }
