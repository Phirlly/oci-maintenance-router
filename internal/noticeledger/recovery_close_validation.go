package noticeledger

import "oci-maintenance-router/internal/delivery"

func validCloseAction(d *recordData, a RecoveryAction) bool {
	if a.Reason != "" || a.Attempt != nil || a.CompletedAt == nil || *a.CompletedAt != a.RecordedAt || len(a.BatchIDs) != 0 || a.Ambiguous || a.InvalidatedAt != nil || a.InvalidationReason != "" {
		return false
	}
	for _, other := range d.Recovery.Actions {
		if other.Request.RequestID != a.Request.RequestID && other.Status == RecoveryClosed && other.Request.Kind == a.Request.Kind && other.Request.TargetID == a.Request.TargetID {
			return false
		}
	}
	if a.Request.Kind == CloseBatch {
		b, ok := d.delivery.Batch(a.Request.TargetID)
		i := revisionIndex(d, b.Message.Revision.Digest)
		return ok && i >= 0 && a.RecordedAt >= d.Revisions[i].FirstSeenAt && b.State == delivery.Closed && reviewTime(b.UpdatedAt) == a.RecordedAt
	}
	if a.Request.Kind != CloseRevision {
		return false
	}
	i := revisionIndex(d, a.Request.TargetID)
	if i < 0 || a.RecordedAt < d.Revisions[i].FirstSeenAt {
		return false
	}
	unfinished := len(d.Revisions[i].Resources) == 0
	for _, row := range d.Revisions[i].Resources {
		unfinished = unfinished || row.BatchID == ""
	}
	for _, b := range d.delivery.Batches() {
		if b.Message.Revision.Digest == a.Request.TargetID {
			if b.State != delivery.Closed && b.State != delivery.APIAccepted {
				return false
			}
			if reviewTime(b.UpdatedAt) > a.RecordedAt {
				return false
			}
			unfinished = unfinished || (b.State == delivery.Closed && reviewTime(b.UpdatedAt) == a.RecordedAt)
		}
	}
	return unfinished
}

func closedBatchCovered(d *recordData, b delivery.BatchSnapshot) bool {
	for _, a := range d.Recovery.Actions {
		if a.Status != RecoveryClosed || a.RecordedAt != reviewTime(b.UpdatedAt) {
			continue
		}
		if (a.Request.Kind == CloseBatch && a.Request.TargetID == b.Message.BatchID) || (a.Request.Kind == CloseRevision && a.Request.TargetID == b.Message.Revision.Digest) {
			return true
		}
	}
	return false
}
