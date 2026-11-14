package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"time"
)

// HoldBatch preserves independent destinations and any existing notice hold.
func (r Record) HoldBatch(id string, reason delivery.Reason, at time.Time) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"invalid_record"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	d.delivery, err = d.delivery.Hold(id, reason, at)
	if err != nil {
		return Record{}, err
	}
	return r.finalize(d, at)
}

// StopUnsent preserves the exact attempt and requires caller evidence of no dispatch.
func (r Record) StopUnsent(ref delivery.AttemptRef, at time.Time) (Record, error) {
	if r.data == nil {
		return Record{}, &Error{"invalid_record"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	d.delivery, err = d.delivery.StopUnsent(ref, at)
	if err != nil {
		return Record{}, err
	}
	return r.finalize(d, at)
}
