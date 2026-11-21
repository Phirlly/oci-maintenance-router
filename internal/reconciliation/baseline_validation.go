package reconciliation

import (
	"regexp"
	"time"
)

var baselineID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var baselineRegion = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var baselineCompartmentID = regexp.MustCompile(`^ocid1\.(compartment|tenancy)\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)
var baselineNoticeID = regexp.MustCompile(`^ocid1\.announcement\.oc1\.\.([A-Za-z0-9_-]*\.)?[A-Za-z0-9_-]+$`)

func validateBaseline(d *baselineData) error {
	if (d.SchemaVersion != 1 && d.SchemaVersion != 2) || !leaseDigest.MatchString(d.EnvironmentID) || !baselineID.MatchString(d.BaselineID) || len(d.Layers) == 0 {
		return &BaselineError{Code: "invalid_schema"}
	}
	completed, ok := leaseTimestamp(d.CompletedAt)
	if !ok {
		return &BaselineError{Code: "invalid_time"}
	}
	d.cells = map[baselineCellKey]int{}
	d.revisions = make([]map[BaselineRevision]bool, 0, len(d.Layers))
	var previous time.Time
	entries := len(d.Layers)
	for i, layer := range d.Layers {
		at, ok := leaseTimestamp(layer.CompletedAt)
		if !ok || at.Before(previous) || at.After(completed) {
			return &BaselineError{Code: "invalid_time"}
		}
		previous = at
		entries += len(layer.Cells) + len(layer.Revisions)
		if entries > baselineEntries {
			return &BaselineError{Code: "too_large"}
		}
		if len(layer.Cells) == 0 || layer.Revisions == nil {
			return &BaselineError{Code: "invalid_scope"}
		}
		for j, cell := range layer.Cells {
			if cell.ResourceType != "" && (d.SchemaVersion != 2 || cell.ResourceType != baselineInstance) {
				return &BaselineError{Code: "invalid_scope"}
			}
			if !baselineRegion.MatchString(cell.Region) || !baselineCompartmentID.MatchString(cell.CompartmentID) || (j > 0 && compareBaselineCells(layer.Cells[j-1], cell) >= 0) {
				return &BaselineError{Code: "invalid_scope"}
			}
			if _, exists := d.cells[cell.key()]; exists {
				return &BaselineError{Code: "overlapping_scope"}
			}
			d.cells[cell.key()] = i
		}
		pairs := map[BaselineRevision]bool{}
		for j, pair := range layer.Revisions {
			if !baselineNoticeID.MatchString(pair.NoticeID) || !leaseDigest.MatchString(pair.Digest) || (j > 0 && compareBaselineRevisions(layer.Revisions[j-1], pair) >= 0) {
				return &BaselineError{Code: "invalid_revision"}
			}
			pairs[pair] = true
		}
		d.revisions = append(d.revisions, pairs)
	}
	if !previous.Equal(completed) {
		return &BaselineError{Code: "invalid_time"}
	}
	if d.SchemaVersion != baselineSchema(d.Layers) {
		return &BaselineError{Code: "invalid_schema"}
	}
	return nil
}
