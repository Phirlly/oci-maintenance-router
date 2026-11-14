package reconciliation

import (
	"context"
	"time"
)

// Initialize is the distinct installer operation. Existing matching history,
// including completed progress, is retained. Ordinary Prepare never initializes.
func (p *BaselinePreparation) Initialize(ctx context.Context, lease *Lease) error {
	r, cancel, err := p.start(ctx, lease, true)
	if err != nil {
		return err
	}
	defer cancel()
	if err = r.initialize(); err != nil {
		return preparationFailure(r.ctx, "initialization_failed", err)
	}
	return nil
}
func (r *preparationRun) initialize() error {
	if err := r.guard(); err != nil {
		return err
	}
	_, err := r.history.Load(r.ctx)
	if err == nil {
		return r.guard()
	}
	if !preparationMissing(r.ctx, err) {
		return err
	}
	if err = r.emptyHistory(); err != nil {
		return err
	}
	if err = r.guard(); err != nil {
		return err
	}
	if _, err = r.history.Create(r.ctx); err != nil {
		return err
	}
	return r.guard()
}
func (r *preparationRun) emptyHistory() error {
	if err := r.reserve(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()
	empty, err := r.owner.objects.HistoryIsEmpty(ctx, preparedHistoryName)
	if err != nil || ctx.Err() != nil {
		failure := preparationFailure(ctx, "history_read", nil)
		failure.HTTPStatus = baselineStorageStatus(err)
		return failure
	}
	if !empty {
		return &PreparationError{Code: "history_retained"}
	}
	return nil
}
