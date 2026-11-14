package noticeledger

import (
	"slices"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func validRetryAction(d *recordData, a RecoveryAction) bool {
	ref := a.Attempt
	if a.Request.Kind != RetryBatch || ref == nil || ref.BatchID != a.Request.TargetID || !recoveryID.MatchString(ref.OwnerToken) || ref.AttemptCount == 0 || !validTime(ref.ExpiresAt) || ref.ExpiresAt <= a.RecordedAt || len(a.BatchIDs) != 0 || a.InvalidatedAt != nil || a.InvalidationReason != "" {
		return false
	}
	start, _ := time.Parse(timeFormat, a.RecordedAt)
	expiry, _ := time.Parse(timeFormat, ref.ExpiresAt)
	if expiry.Sub(start) > 600*time.Second {
		return false
	}
	b, ok := d.delivery.Batch(ref.BatchID)
	if !ok || b.AttemptKind != delivery.ManualAttempt || ref.AttemptCount > b.AttemptCount || a.RecordedAt < reviewTime(b.FirstAttemptAt) || (ref.AttemptCount == 1 && a.RecordedAt != reviewTime(b.FirstAttemptAt)) || (a.Ambiguous && !b.Ambiguous) {
		return false
	}
	current := ref.AttemptCount == b.AttemptCount
	if current && !matchesRecoveryAttempt(a, b) {
		return false
	}
	if a.Status == RecoveryConsumed {
		return current && b.State == delivery.Sending && a.CompletedAt == nil && a.Reason == "" && a.Ambiguous == b.Ambiguous
	}
	if a.CompletedAt == nil || !validTime(*a.CompletedAt) || *a.CompletedAt < a.RecordedAt || *a.CompletedAt > d.UpdatedAt {
		return false
	}
	if a.Status == RecoveryAccepted {
		return current && b.State == delivery.APIAccepted && a.Reason == "" && a.Ambiguous == b.Ambiguous && *a.CompletedAt == reviewTime(b.AcceptedAt) && *a.CompletedAt < ref.ExpiresAt
	}
	if a.Status != RecoveryReviewed || !validRetryResult(a) {
		return false
	}
	if current {
		if b.State == delivery.ReviewRequired {
			return a.Reason == string(b.Reason) && a.Ambiguous == b.Ambiguous && *a.CompletedAt == reviewTime(b.UpdatedAt)
		}
		return b.State == delivery.Closed && a.Ambiguous == b.Ambiguous && *a.CompletedAt <= reviewTime(b.UpdatedAt)
	}
	return true
}

func validRetryResult(a RecoveryAction) bool {
	switch delivery.Reason(a.Reason) {
	case delivery.ReasonNetwork:
		return a.Ambiguous && *a.CompletedAt < a.Attempt.ExpiresAt
	case delivery.ReasonExpired:
		return a.Ambiguous && *a.CompletedAt >= a.Attempt.ExpiresAt
	case delivery.ReasonTransient, delivery.ReasonPermanent, delivery.ReasonOperator:
		return *a.CompletedAt < a.Attempt.ExpiresAt && (a.Attempt.AttemptCount != 1 || !a.Ambiguous)
	}
	return false
}

func validRetryHistory(d *recordData, b delivery.BatchSnapshot) bool {
	actions := []RecoveryAction{}
	for _, a := range d.Recovery.Actions {
		if a.Request.Kind == RetryBatch && a.Request.TargetID == b.Message.BatchID && a.Status != RecoveryRejected {
			actions = append(actions, a)
		}
	}
	if b.AttemptKind != delivery.ManualAttempt {
		return len(actions) == 0
	}
	if len(actions) == 0 {
		return false
	}
	slices.SortFunc(actions, func(a, b RecoveryAction) int {
		if a.Attempt.AttemptCount < b.Attempt.AttemptCount {
			return -1
		}
		if a.Attempt.AttemptCount > b.Attempt.AttemptCount {
			return 1
		}
		return 0
	})
	for i, a := range actions {
		if i == 0 {
			continue
		}
		previous := actions[i-1]
		if previous.Status != RecoveryReviewed || a.Attempt.AttemptCount-1 != previous.Attempt.AttemptCount || a.RecordedAt < *previous.CompletedAt || (previous.Ambiguous && !a.Ambiguous) {
			return false
		}
	}
	return actions[len(actions)-1].Attempt.AttemptCount == b.AttemptCount
}
