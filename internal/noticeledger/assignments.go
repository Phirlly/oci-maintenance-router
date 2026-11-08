package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func effectiveHold(d *recordData, b delivery.BatchSnapshot) delivery.Reason {
	if d.HoldReason != "" {
		return d.HoldReason
	}
	i := revisionIndex(d, b.Message.Revision.Digest)
	if i < 0 {
		return delivery.ReasonSourceChanged
	}
	if d.Revisions[i].RoutingGeneration != b.Message.RoutingGeneration {
		return delivery.ReasonConfigurationChanged
	}
	if d.Revisions[i].Assessment != routing.SupportedShape {
		return delivery.ReasonSourceChanged
	}
	return ""
}

func (r Record) finalize(d recordData, at time.Time) (Record, error) {
	for _, b := range d.delivery.Batches() {
		reason := effectiveHold(&d, b)
		if reason != "" && (b.State == delivery.Prepared || b.State == delivery.RetryWait) {
			var err error
			d.delivery, err = d.delivery.Hold(b.Message.BatchID, reason, at)
			if err != nil {
				return Record{}, err
			}
		}
	}
	if err := synchronize(&d); err != nil {
		return Record{}, err
	}
	if err := capacity(&d); err != nil {
		return Record{}, err
	}
	return Record{data: &d, parent: r.data}, nil
}

func synchronize(d *recordData) error {
	for _, b := range d.delivery.Batches() {
		i := revisionIndex(d, b.Message.Revision.Digest)
		if i < 0 {
			return &Error{"unknown_revision"}
		}
		for _, m := range b.Message.Resources {
			found := false
			for j, row := range d.Revisions[i].Resources {
				if row.ResourceID == m.ResourceID {
					if row.Region != m.Region || row.DestinationID != b.Message.DestinationID || (row.BatchID != "" && row.BatchID != b.Message.BatchID) {
						return &Error{"membership_mismatch"}
					}
					row.BatchID = b.Message.BatchID
					row.Disposition = "ASSIGNED"
					row.Reason = ""
					if b.State == delivery.APIAccepted {
						row.Disposition = "API_ACCEPTED"
					}
					if b.State == delivery.ReviewRequired {
						row.Disposition = "REVIEW_REQUIRED"
						row.Reason = string(b.Reason)
					}
					d.Revisions[i].Resources[j] = row
					found = true
				}
			}
			if !found {
				return &Error{"membership_mismatch"}
			}
		}
	}
	if d.HoldReason != "" {
		for i := range d.Revisions {
			for j, row := range d.Revisions[i].Resources {
				if row.BatchID == "" && row.Disposition != string(routing.OutOfScope) {
					row.Disposition = "REVIEW_REQUIRED"
					row.Reason = string(d.HoldReason)
					row.DestinationID = ""
					d.Revisions[i].Resources[j] = row
				}
			}
		}
	}
	return nil
}
