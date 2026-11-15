package reconciliation

import "time"

// Finish proposes an observed publication outcome. Conditional persistence and
// operator receipt verification remain separate; it never schedules a retry.
func (r OperatorChallenge) Finish(ref OperatorClaimRef, status int, messageID string, at time.Time) (OperatorChallenge, error) {
	if r.data == nil {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_record"}
	}
	d := *r.data
	if d.State != OperatorPublishing {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "terminal"}
	}
	now, err := r.operationTime(at)
	if err != nil {
		return OperatorChallenge{}, err
	}
	expiry, _ := leaseTimestamp(d.ExpiresAt)
	if ref.OwnerToken != d.OwnerToken || ref.ClaimToken != d.ClaimToken || !now.Before(expiry) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "stale_claim"}
	}
	if status != 0 && (status < 100 || status > 599) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_status"}
	}
	if messageID != "" && ((status != 200 && status != 202) || !operatorOpaqueText(messageID, 256)) {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_message_id"}
	}
	d.UpdatedAt = now.Format(leaseTimeFormat)
	d.HTTPStatus, d.MessageID = status, messageID
	d.State, d.Reason, d.Ambiguous = OperatorReview, "response", status < 300 || status >= 500
	switch status {
	case 0:
		d.Reason = "outcome_unknown"
	case 200:
		d.Reason = "response_unverified"
	}
	// Keep202 without a verified ID in its original terminal response shape.
	if messageID != "" {
		d.State, d.Reason, d.Ambiguous = OperatorAccepted, "", false
		accepted := d.UpdatedAt
		d.AcceptedAt = &accepted
	}
	return OperatorChallenge{data: &d, parent: r.data}, nil
}

// Expire cannot determine whether a publication happened. Expired claims require
// review, and neither they nor terminal outcomes ever become publishable again.
func (r OperatorChallenge) Expire(at time.Time) (OperatorChallenge, error) {
	if r.data == nil {
		return OperatorChallenge{}, &OperatorChallengeError{Code: "invalid_record"}
	}
	now, err := r.operationTime(at)
	if err != nil {
		return OperatorChallenge{}, err
	}
	expiry, _ := leaseTimestamp(r.data.ExpiresAt)
	if r.data.State != OperatorPublishing || now.Before(expiry) {
		return r, nil
	}
	d := *r.data
	d.State, d.Reason, d.Ambiguous = OperatorReview, "expired", true
	d.UpdatedAt = now.Format(leaseTimeFormat)
	return OperatorChallenge{data: &d, parent: r.data}, nil
}
func (r OperatorChallenge) operationTime(at time.Time) (time.Time, error) {
	at = at.UTC().Truncate(time.Millisecond)
	updated, _ := leaseTimestamp(r.data.UpdatedAt)
	if !leaseWallTime(at) || at.Before(updated) {
		return time.Time{}, &OperatorChallengeError{Code: "invalid_time"}
	}
	return at, nil
}
