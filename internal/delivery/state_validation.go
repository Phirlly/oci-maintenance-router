package delivery

import "time"

func validState(b savedBatch, version int) bool {
	for _, text := range []*string{b.FirstAttemptAt, b.LastAttemptAt, b.AttemptExpiresAt, b.UpdatedAt, b.NextAttemptAt, b.AcceptedAt} {
		if text != nil {
			v, err := time.Parse(recordTimeFormat, *text)
			if err != nil || v.IsZero() || v.Year() < 1 || v.Year() > 9999 || v.Format(recordTimeFormat) != *text {
				return false
			}
		}
	}
	if b.AttemptCount == 0 {
		if b.OwnerToken != "" || b.FirstAttemptAt != nil || b.LastAttemptAt != nil || b.AttemptExpiresAt != nil || b.NextAttemptAt != nil || b.AcceptedAt != nil || b.Ambiguous {
			return false
		}
		return (b.State == Prepared && b.UpdatedAt == nil && b.Reason == "") || (b.State == ReviewRequired && b.UpdatedAt != nil && holdReason(b.Reason)) || (version == 2 && b.State == Closed && b.UpdatedAt != nil && b.Reason == ReasonOperator)
	}
	if !recordOwner.MatchString(b.OwnerToken) || b.FirstAttemptAt == nil || b.LastAttemptAt == nil || b.AttemptExpiresAt == nil || b.UpdatedAt == nil {
		return false
	}
	first, last, expires, updated := savedTime(b.FirstAttemptAt), savedTime(b.LastAttemptAt), savedTime(b.AttemptExpiresAt), savedTime(b.UpdatedAt)
	manual := lastAttemptKind(b) == ManualAttempt
	if last.Before(first) || updated.Before(last) || (b.AttemptCount == 1 && !first.Equal(last)) || (!manual && !last.Before(first.Add(retryHorizon))) || !expires.After(last) || expires.Sub(last) > maxAttemptLease {
		return false
	}
	firstUnambiguous := b.AttemptCount > 1 || !b.Ambiguous
	switch b.State {
	case Closed:
		return version == 2 && b.NextAttemptAt == nil && b.AcceptedAt == nil && b.Reason == ReasonOperator
	case Sending:
		return updated.Equal(last) && b.NextAttemptAt == nil && b.AcceptedAt == nil && b.Reason == "" && firstUnambiguous
	case APIAccepted:
		return b.AcceptedAt != nil && savedTime(b.AcceptedAt).Equal(updated) && updated.Before(expires) && b.NextAttemptAt == nil && b.Reason == "" && firstUnambiguous
	case RetryWait:
		if manual || b.NextAttemptAt == nil || b.AcceptedAt != nil || savedTime(b.NextAttemptAt).Before(updated) || !savedTime(b.NextAttemptAt).Before(first.Add(retryHorizon)) {
			return false
		}
		switch b.Reason {
		case ReasonTransient:
			return updated.Before(expires) && firstUnambiguous
		case ReasonNetwork:
			return updated.Before(expires) && b.Ambiguous
		case ReasonExpired:
			return !updated.Before(expires) && b.Ambiguous
		}
	case ReviewRequired:
		if b.NextAttemptAt != nil || b.AcceptedAt != nil {
			return false
		}
		if manual {
			return validManualReview(b, updated, expires, firstUnambiguous)
		}
		return (b.Reason == ReasonPermanent && updated.Before(expires) && firstUnambiguous) || b.Reason == ReasonHorizon || holdReason(b.Reason)
	}
	return false
}
