package noticeledger

import (
	"slices"
	"strings"
	"time"

	"oci-maintenance-router/internal/delivery"
)

func (r Record) observationTime(environmentID, noticeID string, at time.Time) (string, error) {
	if r.data != nil && (r.data.EnvironmentID != environmentID || r.data.NoticeID != noticeID) {
		return "", &Error{"identity_mismatch"}
	}
	return r.operationTime(at)
}

func (r Record) mergeObservation(environmentID, noticeID string, v Revision, stamp string, at time.Time) (Record, error) {
	d := recordData{SchemaVersion: 1, EnvironmentID: environmentID, NoticeID: noticeID, Revisions: []Revision{}}
	if r.data != nil {
		d = r.data.copy()
	}
	v.Source = copySource(v.Source)
	v.Resources = slices.Clone(v.Resources)
	v.FirstSeenAt, v.LastSeenAt = stamp, stamp
	i := revisionIndex(&d, v.Digest)
	if i >= 0 {
		previous := d.Revisions[i]
		v.FirstSeenAt = previous.FirstSeenAt
		if len(previous.Resources) != len(v.Resources) {
			return Record{}, &Error{"source_mismatch"}
		}
		for j, old := range previous.Resources {
			if old.BatchID != "" {
				if old.ResourceID != v.Resources[j].ResourceID {
					return Record{}, &Error{"source_mismatch"}
				}
				v.Resources[j] = old
			}
		}
		d.Revisions[i] = v
	} else {
		d.Revisions = append(d.Revisions, v)
	}
	slices.SortFunc(d.Revisions, func(a, b Revision) int { return strings.Compare(a.Digest, b.Digest) })
	d.UpdatedAt = stamp
	if len(d.Revisions) > 1 {
		d.HoldReason = delivery.ReasonSourceChanged
	}
	return r.finalize(d, at)
}

func revisionIndex(d *recordData, digest string) int {
	for i, v := range d.Revisions {
		if v.Digest == digest {
			return i
		}
	}
	return -1
}
