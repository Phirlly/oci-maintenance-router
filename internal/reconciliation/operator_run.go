package reconciliation

import (
	"bytes"
	"context"
)

// Run publishes at most once for a newly confirmed claim. A consumed request is
// never resumed, even under the same lease. The caller releases its own lease.
func (d *OperatorCoordinator) Run(ctx context.Context, lease *Lease) (OperatorResult, error) {
	r, cancel, err := d.start(ctx, lease)
	if err != nil {
		return OperatorResult{}, err
	}
	defer cancel()
	return r.run()
}
func (r *operatorRun) run() (OperatorResult, error) {
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	// Preserve the store's direct-Service404 provenance; public Load deliberately
	// does not grant new-request authority from a missing current object.
	read, cancel, err := operatorStoreContext(r.ctx)
	if err != nil {
		return r.fail(err)
	}
	old, missing, failure := r.store.load(read, r.owner.intent.RequestID)
	cancel()
	if failure == nil {
		return r.existing(old)
	}
	if !missing {
		return r.fail(failure)
	}
	// An unconfirmed or changed subscription must not consume a fresh request.
	if _, err := r.channel(); err != nil {
		return r.result, err
	}
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	_, expiry := r.lease.record.times()
	initial, err := NewOperatorChallenge(r.owner.intent, OperatorClaim{r.lease.record.OwnerToken, r.token, r.lease.manager.now(), expiry})
	if err != nil {
		return r.fail(err)
	}
	saved, err := r.store.Create(r.ctx, initial)
	if err != nil {
		return r.fail(err)
	}
	r.result.record = saved.record
	expected, _ := initial.Encode()
	actual, _ := saved.record.Encode()
	if !bytes.Equal(expected, actual) {
		return r.existing(saved)
	}
	return r.dispatch(saved)
}
func (r *operatorRun) existing(saved StoredOperatorChallenge) (OperatorResult, error) {
	r.result.record = saved.record
	if !saved.record.Matches(r.owner.intent) {
		return r.result, &OperatorRunError{Code: "identity_conflict"}
	}
	if saved.record.data.State != OperatorPublishing {
		return r.result, nil
	}
	next, err := saved.record.Expire(r.lease.manager.now())
	if err != nil {
		return r.fail(err)
	}
	if next.data == saved.record.data {
		return r.result, nil
	}
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	got, err := r.store.Save(r.ctx, saved, next)
	if err != nil {
		return r.fail(err)
	}
	r.result.record = got.record
	return r.result, nil
}
