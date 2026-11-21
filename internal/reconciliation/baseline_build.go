package reconciliation

import (
	"encoding/json"
	"slices"
	"time"
)

// Baseline proposes one immutable initial layer from an actual complete pass.
// Conditional persistence and pinned PREVIEW intent are separate requirements.
func (r DiscoveryResult) Baseline(id string) (Baseline, error) {
	if !r.baselineUsable() {
		return Baseline{}, &BaselineError{Code: "invalid_result"}
	}
	if !baselineID.MatchString(id) {
		return Baseline{}, &BaselineError{Code: "invalid_id"}
	}
	cells, err := r.scope.addedCells(nil)
	if err != nil {
		return Baseline{}, err
	}
	return r.baselineWith(id, nil, cells)
}

func (b Baseline) Extend(id string, r DiscoveryResult) (Baseline, error) {
	if b.data == nil {
		return Baseline{}, &BaselineError{Code: "invalid_record"}
	}
	if !r.baselineUsable() {
		return Baseline{}, &BaselineError{Code: "invalid_result"}
	}
	if b.data.EnvironmentID != r.environmentID {
		return Baseline{}, &BaselineError{Code: "identity_mismatch"}
	}
	if !baselineID.MatchString(id) {
		return Baseline{}, &BaselineError{Code: "invalid_id"}
	}
	if id == b.data.BaselineID {
		return Baseline{}, &BaselineError{Code: "id_reused"}
	}
	if r.completedAt.Before(b.CompletedAt()) {
		return Baseline{}, &BaselineError{Code: "invalid_time"}
	}
	cells, err := r.scope.addedCells(b.data.cells)
	if err != nil {
		return Baseline{}, err
	}
	if len(cells) == 0 {
		return Baseline{}, &BaselineError{Code: "no_new_scope"}
	}
	return r.baselineWith(id, b.data.Layers, cells)
}

func (r DiscoveryResult) baselineWith(id string, previous []BaselineLayer, cells []BaselineCell) (Baseline, error) {
	pairs := []BaselineRevision{}
	for _, match := range r.view.current {
		if match.KnownCandidate {
			pairs = append(pairs, BaselineRevision{match.Revision.NoticeID, match.Revision.Digest})
		}
	}
	slices.SortFunc(pairs, compareBaselineRevisions)
	pairs = slices.Compact(pairs)
	stamp := r.completedAt.UTC().Truncate(time.Millisecond).Format(leaseTimeFormat)
	layers := append(slices.Clone(previous), BaselineLayer{CompletedAt: stamp, Cells: cells, Revisions: pairs})
	d := &baselineData{SchemaVersion: baselineSchema(layers), EnvironmentID: r.environmentID, BaselineID: id, CompletedAt: stamp, Layers: layers}
	if !baselineFits(d) {
		return Baseline{}, &BaselineError{Code: "too_large"}
	}
	if err := validateBaseline(d); err != nil {
		return Baseline{}, err
	}
	return Baseline{data: d}, nil
}

// Sum small, independently encoded parts before serializing the full object.
// Empty-array encodings already account for brackets; add entries and commas.
func baselineFits(d *baselineData) bool {
	header := *d
	header.Layers = []BaselineLayer{}
	encoded, _ := json.Marshal(header)
	size := len(encoded)
	add := func(value any, comma bool) bool {
		part, _ := json.Marshal(value)
		size += len(part)
		if comma {
			size++
		}
		return size <= baselineLimit
	}
	for i, layer := range d.Layers {
		empty := BaselineLayer{CompletedAt: layer.CompletedAt, Cells: []BaselineCell{}, Revisions: []BaselineRevision{}}
		if !add(empty, i > 0) {
			return false
		}
		for j, cell := range layer.Cells {
			if !add(cell, j > 0) {
				return false
			}
		}
		for j, pair := range layer.Revisions {
			if !add(pair, j > 0) {
				return false
			}
		}
	}
	return size <= baselineLimit
}
