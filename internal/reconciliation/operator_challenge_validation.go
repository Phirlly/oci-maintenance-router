package reconciliation

func validOperatorChallengeData(d *operatorChallengeData) bool {
	if d.SchemaVersion != 1 || !validOperatorIntent(d.Intent) || !leaseDigest.MatchString(d.OwnerToken) || !leaseDigest.MatchString(d.ClaimToken) {
		return false
	}
	digest := operatorIntentDigest(d.Intent)
	if d.IntentDigest != digest || d.PayloadDigest != operatorDigest(d.Payload) || d.Payload != operatorChallengePayload(operatorReceiptCode(digest)) {
		return false
	}
	start, okStart := leaseTimestamp(d.StartedAt)
	expiry, okExpiry := leaseTimestamp(d.ExpiresAt)
	updated, okUpdated := leaseTimestamp(d.UpdatedAt)
	if !okStart || !okExpiry || !okUpdated || !expiry.After(start) || expiry.Sub(start) > leaseDuration || updated.Before(start) {
		return false
	}
	switch d.State {
	case OperatorPublishing:
		return d.HTTPStatus == 0 && d.MessageID == "" && d.Reason == "" && !d.Ambiguous && d.AcceptedAt == nil && updated.Equal(start)
	case OperatorAccepted:
		return (d.HTTPStatus == 200 || d.HTTPStatus == 202) && operatorOpaqueText(d.MessageID, 256) && d.Reason == "" && !d.Ambiguous && d.AcceptedAt != nil && *d.AcceptedAt == d.UpdatedAt && updated.Before(expiry)
	case OperatorReview:
		if d.AcceptedAt != nil || d.MessageID != "" {
			return false
		}
		switch d.Reason {
		case "expired":
			return d.HTTPStatus == 0 && d.Ambiguous && !updated.Before(expiry)
		case "outcome_unknown":
			return d.HTTPStatus == 0 && d.Ambiguous && updated.Before(expiry)
		case "response_unverified":
			return d.HTTPStatus == 200 && d.Ambiguous && updated.Before(expiry)
		case "response":
			return d.HTTPStatus >= 100 && d.HTTPStatus <= 599 && d.HTTPStatus != 200 && d.Ambiguous == (d.HTTPStatus < 300 || d.HTTPStatus >= 500) && updated.Before(expiry)
		}
	}
	return false
}
