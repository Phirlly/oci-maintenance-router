package noticeledger

import "oci-maintenance-router/internal/delivery"

// Complete only the currently consumed attempt. Previous results are immutable.
func syncRecoveryOutcomes(d *recordData) error {
	if d.SchemaVersion != 3 {
		return nil
	}
	for i, a := range d.Recovery.Actions {
		if a.Status != RecoveryConsumed {
			continue
		}
		b, ok := d.delivery.Batch(a.Request.TargetID)
		if !ok || a.Attempt == nil || b.AttemptKind != delivery.ManualAttempt || !matchesRecoveryAttempt(a, b) {
			return &Error{"recovery_attempt_mismatch"}
		}
		if b.State == delivery.Sending {
			continue
		}
		if b.State != delivery.APIAccepted && b.State != delivery.ReviewRequired {
			return &Error{"recovery_outcome_mismatch"}
		}
		a.Status = RecoveryReviewed
		if b.State == delivery.APIAccepted {
			a.Status = RecoveryAccepted
		}
		completed := reviewTime(b.UpdatedAt)
		a.Reason, a.Ambiguous, a.CompletedAt = string(b.Reason), b.Ambiguous, &completed
		d.Recovery.Actions[i] = a
	}
	return nil
}

func matchesRecoveryAttempt(a RecoveryAction, b delivery.BatchSnapshot) bool {
	return a.Attempt.BatchID == b.Message.BatchID && a.Attempt.AttemptCount == b.AttemptCount &&
		a.Attempt.OwnerToken == b.OwnerToken && a.RecordedAt == reviewTime(b.LastAttemptAt) &&
		a.Attempt.ExpiresAt == reviewTime(b.AttemptExpiresAt)
}
