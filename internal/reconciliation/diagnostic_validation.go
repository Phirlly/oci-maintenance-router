package reconciliation

func validDiagnosticData(d *diagnosticData) bool {
	if d.SchemaVersion != 1 || !validDiagnosticIntent(d.Intent) || d.SecretVersion <= 0 || !leaseDigest.MatchString(d.OwnerToken) || !leaseDigest.MatchString(d.ClaimToken) {
		return false
	}
	digest, dedup := diagnosticIdentity(d.Intent)
	if d.IntentDigest != digest || d.DedupKey != dedup || d.PayloadDigest != diagnosticDigest(d.Payload) || d.Payload != diagnosticPayload(dedup, diagnosticReceiptCode(digest)) {
		return false
	}
	start, okStart := leaseTimestamp(d.StartedAt)
	expiry, okExpiry := leaseTimestamp(d.ExpiresAt)
	updated, okUpdated := leaseTimestamp(d.UpdatedAt)
	if !okStart || !okExpiry || !okUpdated || !expiry.After(start) || expiry.Sub(start) > leaseDuration || updated.Before(start) {
		return false
	}
	switch d.State {
	case DiagnosticSending:
		return d.HTTPStatus == 0 && d.Reason == "" && !d.Ambiguous && d.AcceptedAt == nil && updated.Equal(start)
	case DiagnosticAccepted:
		return d.HTTPStatus == 202 && d.Reason == "" && !d.Ambiguous && d.AcceptedAt != nil && *d.AcceptedAt == d.UpdatedAt && updated.Before(expiry)
	case DiagnosticReview:
		if d.AcceptedAt != nil {
			return false
		}
		switch d.Reason {
		case "expired":
			return d.HTTPStatus == 0 && d.Ambiguous && !updated.Before(expiry)
		case "outcome_unknown":
			return d.HTTPStatus == 0 && d.Ambiguous && updated.Before(expiry)
		case "response":
			return d.HTTPStatus >= 100 && d.HTTPStatus <= 599 && d.HTTPStatus != 202 && d.Ambiguous == (d.HTTPStatus >= 500) && updated.Before(expiry)
		}
	}
	return false
}
