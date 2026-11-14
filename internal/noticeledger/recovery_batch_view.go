package noticeledger

import (
	"time"

	"oci-maintenance-router/internal/delivery"
)

func recoveryBatchView(b delivery.BatchSnapshot) []any {
	m := b.Message
	resources := make([][2]string, 0, len(m.Resources))
	for _, member := range m.Resources {
		resources = append(resources, [2]string{member.ResourceID, member.Region})
	}
	return []any{m.BatchID, m.Revision.Digest, m.RoutingGeneration, m.DestinationID, m.SecretID,
		b.SecretVersion, m.PagerDutyRegion, resources, m.PayloadDigest, m.DedupKey, b.State,
		b.AttemptCount, b.AttemptKind, b.OwnerToken, reviewTime(b.FirstAttemptAt), reviewTime(b.LastAttemptAt),
		reviewTime(b.AttemptExpiresAt), reviewTime(b.UpdatedAt), reviewTime(b.NextAttemptAt), reviewTime(b.AcceptedAt), b.Reason, b.Ambiguous}
}

func reviewTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(timeFormat)
}
