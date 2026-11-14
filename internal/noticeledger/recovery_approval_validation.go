package noticeledger

import (
	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/routing"
)

func validApprovalAction(d *recordData, a RecoveryAction) bool {
	i := revisionIndex(d, a.Request.TargetID)
	if a.Request.Kind != ApproveRevision || i < 0 || a.RecordedAt < d.Revisions[i].FirstSeenAt || a.Reason != "" || a.Attempt != nil || a.CompletedAt == nil || *a.CompletedAt != a.RecordedAt || a.Ambiguous || len(a.BatchIDs) == 0 || len(a.BatchIDs) > 256 {
		return false
	}
	if a.InvalidatedAt == nil {
		if a.InvalidationReason != "" || d.SourceOutcome.State != SourceAvailable || d.SourceOutcome.RevisionDigest != a.Request.TargetID || d.Revisions[i].RoutingGeneration != a.Request.RoutingGeneration || d.Revisions[i].Assessment != routing.SupportedShape {
			return false
		}
	} else if !validTime(*a.InvalidatedAt) || *a.InvalidatedAt < a.RecordedAt || *a.InvalidatedAt > d.UpdatedAt || !holdReason(delivery.Reason(a.InvalidationReason)) {
		return false
	}
	previous := ""
	for _, id := range a.BatchIDs {
		b, ok := d.delivery.Batch(id)
		if id <= previous || !ok || b.Message.Revision.Digest != a.Request.TargetID || b.Message.RoutingGeneration != a.Request.RoutingGeneration || (!b.LastAttemptAt.IsZero() && reviewTime(b.FirstAttemptAt) < a.RecordedAt) {
			return false
		}
		previous = id
	}
	return true
}
