package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

// Prepare pins only new READY members. Existing batches retain their first
// bytes and secret version. This is not source freshness or send authorization.
func (r Record) Prepare(at time.Time, values ...delivery.Preparation) (Record, error) {
	if r.data == nil || r.data.HoldReason != "" {
		return Record{}, &Error{"preparation_blocked"}
	}
	stamp, err := r.operationTime(at)
	if err != nil {
		return Record{}, err
	}
	if len(values) > 256 {
		return Record{}, &Error{"too_many_preparations"}
	}
	for _, p := range values {
		s := p.Draft.Snapshot()
		if p.SecretVersion <= 0 || s.BatchID == "" || s.EnvironmentID != r.data.EnvironmentID || s.Revision.NoticeID != r.data.NoticeID {
			return Record{}, &Error{"invalid_preparation"}
		}
		if _, ok := r.data.delivery.Batch(s.BatchID); ok {
			continue
		}
		i := revisionIndex(r.data, s.Revision.Digest)
		if i < 0 {
			return Record{}, &Error{"unknown_revision"}
		}
		v := r.data.Revisions[i]
		if v.Assessment != routing.SupportedShape || v.RoutingGeneration != s.RoutingGeneration {
			return Record{}, &Error{"stale_preparation"}
		}
		for _, member := range s.Resources {
			found := false
			for _, d := range v.Resources {
				if d.ResourceID == member.ResourceID {
					found = d.BatchID == "" && d.Disposition == string(routing.Ready) && d.Region == member.Region && d.DestinationID == s.DestinationID
				}
			}
			if !found {
				return Record{}, &Error{"member_not_ready"}
			}
		}
	}
	d := r.data.copy()
	d.UpdatedAt = stamp
	if len(values) != 0 {
		if len(d.delivery.Batches()) == 0 {
			d.delivery, err = delivery.NewRecord(values...)
		} else {
			d.delivery, err = d.delivery.Prepare(values...)
		}
		if err != nil {
			return Record{}, err
		}
	}
	return r.finalize(d, at)
}
