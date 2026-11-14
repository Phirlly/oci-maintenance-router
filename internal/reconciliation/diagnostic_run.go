package reconciliation

import (
	"bytes"
	"context"
	"oci-maintenance-router/internal/ocisecrets"
)

// Run performs at most one diagnostic adapter call. Repeating a consumed request
// never dispatches again, even under the same lease. The caller releases its lease.
func (d *DiagnosticCoordinator) Run(ctx context.Context, lease *Lease) (DiagnosticResult, error) {
	r, cancel, err := d.start(ctx, lease)
	if err != nil {
		return DiagnosticResult{}, err
	}
	defer cancel()
	return r.run()
}
func (r *diagnosticRun) run() (DiagnosticResult, error) {
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	// Public Load intentionally drops missing provenance. Retain the store's
	// exact direct-Service404 classification under its ordinary bounded context.
	read, cancel, err := diagnosticStoreContext(r.ctx)
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
	if err := r.reserve(); err != nil {
		return r.fail(err)
	}
	secretCtx, stop := context.WithTimeout(r.ctx, discoveryReserve)
	credential, err := r.owner.current(secretCtx, r.owner.intent.SecretID)
	late := secretCtx.Err()
	stop()
	if err != nil || late != nil {
		failure := diagnosticRunError(r.ctx, "secret_read", nil)
		if e, ok := err.(*ocisecrets.Error); ok && e != nil && e.Status >= 100 && e.Status <= 599 {
			failure.HTTPStatus = e.Status
		}
		return r.result, failure
	}
	if credential == nil || credential.Version() <= 0 {
		return r.result, &DiagnosticRunError{Code: "secret_invalid"}
	}
	if err := r.guard(); err != nil {
		return r.fail(err)
	}
	_, expiry := r.lease.record.times()
	initial, err := NewDiagnostic(r.owner.intent, credential.Version(), DiagnosticClaim{r.lease.record.OwnerToken, r.token, r.lease.manager.now(), expiry})
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
	return r.dispatch(saved, credential)
}
func (r *diagnosticRun) existing(saved StoredDiagnostic) (DiagnosticResult, error) {
	r.result.record = saved.record
	if !saved.record.Matches(r.owner.intent) {
		return r.result, &DiagnosticRunError{Code: "identity_conflict"}
	}
	if saved.record.data.State != DiagnosticSending {
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
