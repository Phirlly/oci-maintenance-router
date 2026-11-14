package reconciliation

import (
	"encoding/json"
	"slices"
	"time"

	"oci-maintenance-router/internal/notices"
)

// Baseline is an immutable proposal or validated stored representation. Neither
// grants selection, activation or send authority.
type Baseline struct{ data *baselineData }

type BaselineCell struct {
	Region        string `json:"region"`
	CompartmentID string `json:"compartmentId"`
}
type BaselineRevision struct {
	NoticeID string `json:"noticeId"`
	Digest   string `json:"revisionDigest"`
}
type BaselineLayer struct {
	CompletedAt string             `json:"completedAt"`
	Cells       []BaselineCell     `json:"cells"`
	Revisions   []BaselineRevision `json:"revisions"`
}
type BaselineError struct{ Code string }

func (e *BaselineError) Error() string { return "baseline: " + e.Code }

type baselineData struct {
	SchemaVersion int             `json:"schemaVersion"`
	EnvironmentID string          `json:"environmentId"`
	BaselineID    string          `json:"baselineId"`
	CompletedAt   string          `json:"completedAt"`
	Layers        []BaselineLayer `json:"layers"`
	cells         map[BaselineCell]int
	revisions     []map[BaselineRevision]bool
}

func (b Baseline) ID() string {
	if b.data == nil {
		return ""
	}
	return b.data.BaselineID
}
func (b Baseline) EnvironmentID() string {
	if b.data == nil {
		return ""
	}
	return b.data.EnvironmentID
}
func (b Baseline) CompletedAt() time.Time {
	if b.data == nil {
		return time.Time{}
	}
	at, _ := leaseTimestamp(b.data.CompletedAt)
	return at
}
func (b Baseline) Layers() []BaselineLayer {
	if b.data == nil {
		return nil
	}
	layers := slices.Clone(b.data.Layers)
	for i := range layers {
		layers[i].Cells = slices.Clone(layers[i].Cells)
		layers[i].Revisions = slices.Clone(layers[i].Revisions)
	}
	return layers
}
func (b Baseline) Encode() ([]byte, error) {
	if b.data == nil {
		return nil, &BaselineError{Code: "invalid_record"}
	}
	return json.Marshal(b.data)
}

// Suppresses applies only within the layer owning this exact cell. It neither
// validates current resource metadata nor authorizes activation or sending.
func (b Baseline) Suppresses(region, compartment string, revision notices.RevisionKey) bool {
	if b.data == nil {
		return false
	}
	layer, ok := b.data.cells[BaselineCell{region, compartment}]
	return ok && b.data.revisions[layer][BaselineRevision{revision.NoticeID, revision.Digest}]
}

// Covers requires complete same-environment discovery. It does not select this
// baseline or prove that the current controls still match that completed pass.
func (b Baseline) Covers(r DiscoveryResult) bool {
	if b.data == nil || !r.baselineUsable() || b.data.EnvironmentID != r.environmentID {
		return false
	}
	if len(r.scope.regions) > len(b.data.cells)/len(r.scope.compartments) {
		return false
	}
	for _, region := range r.scope.regions {
		for _, compartment := range r.scope.compartments {
			if _, ok := b.data.cells[BaselineCell{region, compartment}]; !ok {
				return false
			}
		}
	}
	return true
}
