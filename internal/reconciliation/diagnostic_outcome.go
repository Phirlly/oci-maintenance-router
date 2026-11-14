package reconciliation

import "time"

// Finish records an observed result as a proposal. A current owner must still
// confirm conditional persistence; 202 alone is neither durability nor receipt.
func (r DiagnosticRecord) Finish(ref DiagnosticClaimRef, status int, at time.Time) (DiagnosticRecord, error) {
	if r.data == nil {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_record"}
	}
	d := *r.data
	if d.State != DiagnosticSending {
		return DiagnosticRecord{}, &DiagnosticError{Code: "terminal"}
	}
	now, err := r.operationTime(at)
	if err != nil {
		return DiagnosticRecord{}, err
	}
	expiry, _ := leaseTimestamp(d.ExpiresAt)
	if ref.OwnerToken != d.OwnerToken || ref.ClaimToken != d.ClaimToken || !now.Before(expiry) {
		return DiagnosticRecord{}, &DiagnosticError{Code: "stale_claim"}
	}
	if status != 0 && (status < 100 || status > 599) {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_status"}
	}
	d.UpdatedAt = now.Format(leaseTimeFormat)
	d.HTTPStatus = status
	d.State, d.Reason, d.Ambiguous = DiagnosticReview, "response", status == 0 || status >= 500
	if status == 0 {
		d.Reason = "outcome_unknown"
	}
	if status == 202 {
		d.State, d.Reason, d.Ambiguous = DiagnosticAccepted, "", false
		accepted := d.UpdatedAt
		d.AcceptedAt = &accepted
	}
	return DiagnosticRecord{data: &d, parent: r.data}, nil
}

// Expire never assumes a claimed request was unsent and never makes it retryable.
func (r DiagnosticRecord) Expire(at time.Time) (DiagnosticRecord, error) {
	if r.data == nil {
		return DiagnosticRecord{}, &DiagnosticError{Code: "invalid_record"}
	}
	now, err := r.operationTime(at)
	if err != nil {
		return DiagnosticRecord{}, err
	}
	expiry, _ := leaseTimestamp(r.data.ExpiresAt)
	if r.data.State != DiagnosticSending || now.Before(expiry) {
		return r, nil
	}
	d := *r.data
	d.State, d.Reason, d.Ambiguous = DiagnosticReview, "expired", true
	d.UpdatedAt = now.Format(leaseTimeFormat)
	return DiagnosticRecord{data: &d, parent: r.data}, nil
}
func (r DiagnosticRecord) operationTime(at time.Time) (time.Time, error) {
	at = at.UTC().Truncate(time.Millisecond)
	updated, _ := leaseTimestamp(r.data.UpdatedAt)
	if !leaseWallTime(at) || at.Before(updated) {
		return time.Time{}, &DiagnosticError{Code: "invalid_time"}
	}
	return at, nil
}
