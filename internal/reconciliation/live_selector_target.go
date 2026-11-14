package reconciliation

func (r *liveSelectionRun) target() (Baseline, error) {
	saved, err := r.history.Load(r.ctx)
	if err != nil {
		return Baseline{}, err
	}
	h := saved.History()
	if h.BaselineID() == "" {
		return Baseline{}, &LiveSelectionError{Code: "baseline_unprepared"}
	}
	b, err := r.baselines.Load(r.ctx, h.BaselineID())
	if err != nil {
		return Baseline{}, err
	}
	if referenceForBaseline(b).Digest != h.BaselineDigest() {
		return Baseline{}, &LiveSelectionError{Code: "history_mismatch"}
	}
	cells, err := r.owner.scope.addedCells(b.data.cells)
	if err != nil {
		return Baseline{}, err
	}
	if len(cells) != 0 {
		return Baseline{}, &LiveSelectionError{Code: "scope_unprepared"}
	}
	if err = r.checkTime(b); err != nil {
		return Baseline{}, err
	}
	return b, nil
}

func (r *liveSelectionRun) selectTarget(target Baseline) error {
	// Keep the store's direct-Service404 provenance inside its ordinary10s cap.
	// Neither a public status code nor a malformed response authorizes Create.
	read, cancel, err := baselineStoreContext(r.ctx)
	if err != nil {
		return err
	}
	old, missing, failure := r.selected.load(read)
	cancel()
	if failure != nil {
		if !missing {
			return failure
		}
		initial, err := NewBaselineSelection(r.selected.environment, r.selected.installation, target)
		if err != nil {
			return err
		}
		if err = r.guard(); err != nil {
			return err
		}
		_, err = r.selected.Create(r.ctx, initial)
		return err
	}
	selection := old.Selection()
	if selection.Matches(target) {
		return nil
	}
	previous, err := r.baselines.Load(r.ctx, selection.BaselineID())
	if err != nil {
		return err
	}
	if !selection.Matches(previous) {
		return &LiveSelectionError{Code: "selection_mismatch"}
	}
	next, err := selection.Advance(previous, target)
	if err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	_, err = r.selected.Save(r.ctx, old, next)
	return err
}
