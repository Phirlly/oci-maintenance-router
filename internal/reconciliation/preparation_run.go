package reconciliation

import (
	"context"
	"slices"
)

// Prepare confirms cumulative private history. Covered scope returns its original
// baseline, not a new scan or activation evidence. The caller releases the lease.
func (p *BaselinePreparation) Prepare(ctx context.Context, lease *Lease) (Baseline, error) {
	r, cancel, err := p.start(ctx, lease, false)
	if err != nil {
		return Baseline{}, err
	}
	defer cancel()
	b, err := r.prepare()
	if err != nil {
		return Baseline{}, preparationFailure(r.ctx, "prepare_failed", err)
	}
	return b, nil
}
func (r *preparationRun) prepare() (Baseline, error) {
	if err := r.guard(); err != nil {
		return Baseline{}, err
	}
	saved, err := r.history.Load(r.ctx)
	if err != nil {
		return Baseline{}, err
	}
	h := saved.History()
	previous := Baseline{}
	var existing map[BaselineCell]int
	if h.BaselineID() != "" {
		previous, err = r.baselines.Load(r.ctx, h.BaselineID())
		if err != nil {
			return Baseline{}, err
		}
		if referenceForBaseline(previous).Digest != h.BaselineDigest() {
			return Baseline{}, &PreparationError{Code: "history_mismatch"}
		}
		existing = previous.data.cells
	}
	cells, err := r.owner.discovery.scope.addedCells(existing)
	if err != nil {
		return Baseline{}, err
	}
	if len(cells) == 0 {
		if err := r.guard(); err != nil {
			return Baseline{}, err
		}
		return previous, nil
	}
	id := r.owner.targetID(h)
	target, err := r.baselines.Load(r.ctx, id)
	if err != nil {
		if !preparationMissing(r.ctx, err) {
			return Baseline{}, err
		}
		target, err = r.build(previous, id)
		if err != nil {
			return Baseline{}, err
		}
	}
	next, err := h.Advance(previous, target)
	if err != nil || !slices.Equal(target.data.Layers[len(target.data.Layers)-1].Cells, cells) {
		return Baseline{}, &PreparationError{Code: "target_mismatch"}
	}
	if err := r.guard(); err != nil {
		return Baseline{}, err
	}
	if _, err := r.history.Save(r.ctx, saved, next); err != nil {
		return Baseline{}, err
	}
	if err := r.guard(); err != nil {
		return Baseline{}, err
	}
	return target, nil
}
func (r *preparationRun) build(previous Baseline, id string) (Baseline, error) {
	result, err := r.owner.discovery.Run(r.ctx, r.lease)
	if err != nil {
		return Baseline{}, err
	}
	var target Baseline
	if previous.data == nil {
		target, err = result.Baseline(id)
	} else {
		target, err = previous.Extend(id, result)
	}
	if err != nil {
		return Baseline{}, err
	}
	if err := r.guard(); err != nil {
		return Baseline{}, err
	}
	return r.baselines.Create(r.ctx, target)
}
