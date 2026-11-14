package reconciliation

import (
	"crypto/sha256"
	"fmt"
	"slices"
)

// Advance binds a proposal to this exact head and one cumulative layer. The
// coordinator must separately verify persisted target, requested scope, current
// controls and lease; an in-memory baseline is not evidence of those gates.
func (h PreparedHistory) Advance(previous, next Baseline) (PreparedHistory, error) {
	if h.data == nil || next.data == nil || next.EnvironmentID() != h.EnvironmentID() {
		return PreparedHistory{}, &BaselineError{Code: "invalid_record"}
	}
	if h.data.Prepared == nil {
		if previous.data != nil || len(next.data.Layers) != 1 {
			return PreparedHistory{}, &BaselineError{Code: "invalid_transition"}
		}
	} else {
		if previous.data == nil || preparedReference(previous) != *h.data.Prepared ||
			previous.EnvironmentID() != h.EnvironmentID() || next.ID() == previous.ID() ||
			len(next.data.Layers) != len(previous.data.Layers)+1 {
			return PreparedHistory{}, &BaselineError{Code: "invalid_transition"}
		}
		for i, old := range previous.data.Layers {
			added := next.data.Layers[i]
			if old.CompletedAt != added.CompletedAt || !slices.Equal(old.Cells, added.Cells) || !slices.Equal(old.Revisions, added.Revisions) {
				return PreparedHistory{}, &BaselineError{Code: "invalid_transition"}
			}
		}
	}
	d := *h.data
	reference := preparedReference(next)
	d.Prepared = &reference
	return PreparedHistory{data: &d, parent: h.data}, nil
}

func preparedReference(b Baseline) preparedBaselineReference {
	body, _ := b.Encode()
	return preparedBaselineReference{b.ID(), fmt.Sprintf("%x", sha256.Sum256(body))}
}
