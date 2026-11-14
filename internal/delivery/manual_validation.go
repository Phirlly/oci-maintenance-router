package delivery

import "time"

func validManualReview(b savedBatch, updated, expires time.Time, firstUnambiguous bool) bool {
	switch b.Reason {
	case ReasonTransient, ReasonPermanent, ReasonOperator:
		return updated.Before(expires) && firstUnambiguous
	case ReasonNetwork:
		return updated.Before(expires) && b.Ambiguous
	case ReasonExpired:
		return !updated.Before(expires) && b.Ambiguous
	}
	return false
}
