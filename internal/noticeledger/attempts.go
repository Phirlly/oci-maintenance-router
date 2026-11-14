package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
)

func (r Record) Begin(id string, c delivery.AttemptContext) (Record, delivery.AttemptRef, error) {
	if r.data == nil || r.data.HoldReason != "" {
		return Record{}, delivery.AttemptRef{}, &Error{"attempt_blocked"}
	}
	stamp, err := r.operationTime(c.Now)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	b, ok := r.data.delivery.Batch(id)
	_, approved := recoveryApproval(r.data, id)
	if !ok || approved || effectiveHold(r.data, b) != "" {
		return Record{}, delivery.AttemptRef{}, &Error{"attempt_blocked"}
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	var ref delivery.AttemptRef
	d.delivery, ref, err = d.delivery.Begin(id, c)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	next, err := r.finalize(d, c.Now)
	if err != nil {
		return Record{}, delivery.AttemptRef{}, err
	}
	return next, ref, nil
}

func (r Record) Finish(ref delivery.AttemptRef, status int, at, retryAt time.Time) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"invalid_record"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	d.delivery, err = d.delivery.Finish(ref, status, at, retryAt)
	if err != nil {
		return Record{}, err
	}
	return r.finalize(d, at)
}

func (r Record) Recover(at time.Time) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"invalid_record"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	if len(d.delivery.Batches()) != 0 {
		d.delivery, err = d.delivery.Recover(at)
		if err != nil {
			return Record{}, err
		}
	}
	return r.finalize(d, at)
}

func (r Record) Hold(reason delivery.Reason, at time.Time) (Record, error) {
	if r.data == nil || !holdReason(reason) {
		return Record{}, &Error{"invalid_hold"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	if d.HoldReason == "" {
		d.HoldReason = reason
	}
	invalidateApprovals(&d, string(reason))
	return r.finalize(d, at)
}

func holdReason(reason delivery.Reason) bool {
	switch reason {
	case delivery.ReasonSourceChanged, delivery.ReasonSourceUnavailable, delivery.ReasonConfigurationChanged, delivery.ReasonSecretUnavailable, delivery.ReasonOperator:
		return true
	}
	return false
}
