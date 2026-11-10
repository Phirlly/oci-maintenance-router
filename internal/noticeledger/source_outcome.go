package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
)

type SourceState string

const (
	SourceUnknown     SourceState = "UNKNOWN"
	SourceAvailable   SourceState = "AVAILABLE"
	SourceUnavailable SourceState = "UNAVAILABLE"
)

// SourceOutcome records evidence at a ledger transition, not fetch freshness.
// It grants no discovery, activation or delivery authority.
type SourceOutcome struct {
	State          SourceState `json:"state"`
	RecordedAt     string      `json:"recordedAt"`
	RevisionDigest string      `json:"revisionDigest"`
	HTTPStatus     int         `json:"httpStatus"`
}

// MigrateSourceOutcome proposes one explicit v1-to-v2 transition. Save it before
// proposing another transition. Saved observations do not imply availability.
func (r Record) MigrateSourceOutcome(at time.Time) (Record, error) {
	if r.data == nil || r.data.SchemaVersion != 1 {
		return Record{}, &Error{"invalid_migration"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.SchemaVersion, d.UpdatedAt = 2, stamp
	d.SourceOutcome = &SourceOutcome{State: SourceUnknown, RecordedAt: stamp}
	return r.finalize(d, at)
}

// RecordSourceUnavailable retains a controlled outcome and imposes a sticky
// hold in one proposal. A prior hold and active delivery results remain intact.
func (r Record) RecordSourceUnavailable(status int, at time.Time) (Record, error) {
	if r.data == nil || !unavailableStatus(status) {
		return Record{}, &Error{"invalid_source_outcome"}
	}
	if r.data.SchemaVersion != 2 {
		return Record{}, &Error{"migration_required"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	d.SourceOutcome = &SourceOutcome{State: SourceUnavailable, RecordedAt: stamp, HTTPStatus: status}
	if d.HoldReason == "" {
		d.HoldReason = delivery.ReasonSourceUnavailable
	}
	return r.finalize(d, at)
}

func unavailableStatus(status int) bool {
	switch status {
	case 401, 403, 404, 408, 429, 500, 502, 503, 504:
		return true
	}
	return false
}
