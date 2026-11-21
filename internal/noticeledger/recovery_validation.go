package noticeledger

import "oci-maintenance-router/internal/delivery"

func validRecovery(d *recordData) bool {
	if !recoveryVersion(d.SchemaVersion) {
		return d.Recovery == nil
	}
	if d.Recovery == nil || d.Recovery.Actions == nil || d.SourceOutcome == nil {
		return false
	}
	previous := ""
	approved := map[string]bool{}
	for _, a := range d.Recovery.Actions {
		if !validRecoveryRequest(a.Request) || a.Request.RequestID <= previous || a.IntentDigest != recoveryIntentDigest(d, a.Request) || !validTime(a.RecordedAt) || a.RecordedAt > d.UpdatedAt || a.BatchIDs == nil {
			return false
		}
		previous = a.Request.RequestID
		if !validRecoveryAction(d, a) {
			return false
		}
		if a.Status == RecoveryApproved {
			for _, id := range a.BatchIDs {
				if approved[id] {
					return false
				}
				approved[id] = true
			}
		}
	}
	for _, b := range d.delivery.Batches() {
		if !validRetryHistory(d, b) || (b.State == delivery.Closed && !closedBatchCovered(d, b)) {
			return false
		}
	}
	return true
}

func validRecoveryAction(d *recordData, a RecoveryAction) bool {
	if a.Status == RecoveryRejected {
		return recoveryRejection(a.Reason) && a.CompletedAt != nil && *a.CompletedAt == a.RecordedAt && a.Attempt == nil && len(a.BatchIDs) == 0 && a.InvalidatedAt == nil && a.InvalidationReason == "" && !a.Ambiguous
	}
	if a.Status == RecoveryApproved {
		return validApprovalAction(d, a)
	}
	if a.Status == RecoveryClosed {
		return validCloseAction(d, a)
	}
	return validRetryAction(d, a)
}
