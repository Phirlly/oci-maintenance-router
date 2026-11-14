package reconciliation

import "slices"

// Advance proposes one selection transition, which may span multiple prepared
// extensions. The previously selected layers must remain completely unchanged.
// Exact repeats use Matches instead; they need no write and grant no authority.
func (s BaselineSelection) Advance(previous, next Baseline) (BaselineSelection, error) {
	if !s.Matches(previous) || next.data == nil || next.EnvironmentID() != s.EnvironmentID() || next.ID() == previous.ID() || len(next.data.Layers) <= len(previous.data.Layers) {
		return BaselineSelection{}, &BaselineError{Code: "invalid_transition"}
	}
	for i, old := range previous.data.Layers {
		layer := next.data.Layers[i]
		if old.CompletedAt != layer.CompletedAt || !slices.Equal(old.Cells, layer.Cells) || !slices.Equal(old.Revisions, layer.Revisions) {
			return BaselineSelection{}, &BaselineError{Code: "invalid_transition"}
		}
	}
	d := *s.data
	ref := referenceForBaseline(next)
	d.Selected = &ref
	return BaselineSelection{data: &d, parent: s.data}, nil
}
