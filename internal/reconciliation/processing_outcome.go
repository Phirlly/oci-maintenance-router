package reconciliation

import (
	"context"
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/pagerduty"
	"time"
)

func (r *processingRun) dispatch(saved ocistorage.StoredNotice, b delivery.BatchSnapshot, ref delivery.AttemptRef, key []byte) (ocistorage.StoredNotice, error) {
	index := len(r.result.snapshot.Attempts)
	r.textBytes += len(b.Message.Revision.NoticeID) + len(b.Message.BatchID)
	r.result.snapshot.Attempts = append(r.result.snapshot.Attempts, ProcessingAttempt{NoticeID: b.Message.Revision.NoticeID, BatchID: b.Message.BatchID, MayHaveSent: true})
	response, failure := r.owner.senders[b.Message.PagerDutyRegion](r.ctx, b.Message.PayloadWithoutRoutingKey, key)
	status := response.Status
	if status < 100 || status > 599 {
		status = 0
	}
	r.result.snapshot.Attempts[index].Status = status
	unsent := status == 0 && knownUnsent(failure)
	r.result.snapshot.Attempts[index].MayHaveSent = !unsent
	at := r.lease.manager.now()
	var next noticeledger.Record
	var err error
	if unsent {
		next, err = saved.Record().StopUnsent(ref, at)
	} else {
		current, _ := processingBatch(saved, b.Message.BatchID)
		var retryAt time.Time
		if current.AttemptKind != delivery.ManualAttempt {
			var retryErr error
			retryAt, retryErr = r.retryAt(current, response, status, at)
			if retryErr != nil {
				return saved, retryErr
			}
		}
		next, err = saved.Record().Finish(ref, status, at, retryAt)
	}
	if err != nil {
		return saved, &ProcessingError{Code: "outcome_invalid", WriteUncertain: true}
	}
	receipt, cancel := context.WithDeadline(context.WithoutCancel(r.ctx), r.deadline)
	defer cancel()
	reserve := r.storage.reserve
	r.storage.reserve = func() error {
		if receipt.Err() != nil {
			return processingFailure(receipt, &ProcessingError{Code: "outcome_unconfirmed", WriteUncertain: true})
		}
		now := r.lease.manager.now()
		if !leaseWallTime(now) || now.Before(at) || !now.Before(r.deadline) {
			return &ProcessingError{Code: "outcome_unconfirmed", WriteUncertain: true}
		}
		return nil
	}
	defer func() { r.storage.reserve = reserve }()
	got, err := r.write(receipt, saved, next)
	if err != nil {
		return saved, &ProcessingError{Code: "outcome_unconfirmed", WriteUncertain: true}
	}
	r.result.snapshot.Attempts[index].Confirmed = true
	r.captureRecovery(got)
	if status == 202 {
		r.result.snapshot.Accepted++
	}
	return got, nil
}

func knownUnsent(err error) bool {
	e, ok := err.(*pagerduty.Error)
	if !ok || e == nil || e.MayHaveSent {
		return false
	}
	switch e.Code {
	case pagerduty.InvalidConfiguration, pagerduty.InvalidRequest, pagerduty.DeadlineRequired, pagerduty.Canceled, pagerduty.DeadlineExceeded, pagerduty.Transport, pagerduty.TooLarge:
		return true
	}
	return false
}

func (r *processingRun) retryAt(b delivery.BatchSnapshot, response pagerduty.Response, status int, at time.Time) (time.Time, error) {
	if status != 0 && status != 429 && status < 500 {
		return time.Time{}, nil
	}
	horizon := b.FirstAttemptAt.Add(24 * time.Hour)
	// An attempt may begin before the horizon and finish after it. Finish still
	// requires a monotonic proposed retry timestamp to persist horizon review.
	boundary := horizon
	if at.After(boundary) {
		boundary = at
	}
	if response.RetryAfterOverflow {
		return boundary, nil
	}
	limit := 30 * time.Second
	minimum := time.Duration(0)
	if status == 429 {
		minimum = 180 * time.Second
	} else if b.AttemptCount > 0 && b.AttemptCount <= 5 {
		limit = time.Second << uint(b.AttemptCount-1)
	}
	jitter := r.owner.jitter(limit)
	if jitter < 0 || jitter > limit {
		return time.Time{}, &ProcessingError{Code: "invalid_jitter"}
	}
	next := at.Add(minimum + jitter)
	if response.RetryAfter.After(next) {
		next = response.RetryAfter
	}
	// Preserve a representable boundary that Finish converts into horizon review.
	if !next.Before(horizon) {
		return boundary, nil
	}
	return next, nil
}
