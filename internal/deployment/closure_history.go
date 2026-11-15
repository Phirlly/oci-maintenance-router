package deployment

import (
	"time"

	"oci-maintenance-router/internal/delivery"
	"oci-maintenance-router/internal/noticeledger"
)

type closureBatchItem struct {
	Kind              string `json:"kind"`
	RevisionID        string `json:"revisionId"`
	BatchID           string `json:"batchId"`
	RoutingGeneration string `json:"routingGeneration"`
	DestinationID     string `json:"destinationId"`
	State             string `json:"state"`
	Reason            string `json:"reason"`
	Blocker           string `json:"blocker"`
	Ambiguous         bool   `json:"ambiguous"`
	AttemptCount      uint64 `json:"attemptCount"`
	FirstAttemptAt    string `json:"firstAttemptAt"`
	LastAttemptAt     string `json:"lastAttemptAt"`
	AttemptExpiresAt  string `json:"attemptExpiresAt"`
	NextAttemptAt     string `json:"nextAttemptAt"`
	AcceptedAt        string `json:"acceptedAt"`
	UpdatedAt         string `json:"updatedAt"`
}

func closureStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func closureBatch(record noticeledger.Record, b delivery.BatchSnapshot) closureBatchItem {
	blocker, _ := record.ClosureBlocker(noticeledger.CloseBatch, b.Message.BatchID)
	return closureBatchItem{"BATCH", b.Message.Revision.Digest, b.Message.BatchID, b.Message.RoutingGeneration, b.Message.DestinationID,
		string(b.State), string(b.Reason), blocker, b.Ambiguous, b.AttemptCount, closureStamp(b.FirstAttemptAt), closureStamp(b.LastAttemptAt),
		closureStamp(b.AttemptExpiresAt), closureStamp(b.NextAttemptAt), closureStamp(b.AcceptedAt), closureStamp(b.UpdatedAt)}
}

type closureActionItem struct {
	Kind               string  `json:"kind"`
	RequestID          string  `json:"requestId"`
	Action             string  `json:"action"`
	TargetID           string  `json:"targetId"`
	RoutingGeneration  string  `json:"routingGeneration"`
	Status             string  `json:"status"`
	Reason             string  `json:"reason"`
	RecordedAt         string  `json:"recordedAt"`
	CompletedAt        *string `json:"completedAt"`
	InvalidatedAt      *string `json:"invalidatedAt"`
	InvalidationReason string  `json:"invalidationReason"`
	Ambiguous          bool    `json:"ambiguous"`
}

type closureActionBatchItem struct {
	Kind      string `json:"kind"`
	RequestID string `json:"requestId"`
	BatchID   string `json:"batchId"`
}

func closureHistory(saved *noticeledger.RecoverySnapshot) []any {
	items := []any{}
	for _, a := range saved.Actions {
		items = append(items, closureActionItem{"ACTION", a.Request.RequestID, string(a.Request.Kind), a.Request.TargetID, a.Request.RoutingGeneration,
			string(a.Status), a.Reason, a.RecordedAt, a.CompletedAt, a.InvalidatedAt, a.InvalidationReason, a.Ambiguous})
		for _, id := range a.BatchIDs {
			items = append(items, closureActionBatchItem{"ACTION_BATCH", a.Request.RequestID, id})
		}
	}
	return items
}
