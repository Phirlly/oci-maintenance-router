package deployment

import "oci-maintenance-router/internal/noticeledger"

type closureNoticeItem struct {
	Kind             string `json:"kind"`
	NoticeID         string `json:"noticeId"`
	HoldReason       string `json:"holdReason"`
	SourceState      string `json:"sourceState"`
	SourceRevision   string `json:"sourceRevision"`
	SourceStatus     int    `json:"sourceStatus"`
	SourceRecordedAt string `json:"sourceRecordedAt"`
}

type closureRevisionItem struct {
	Kind              string  `json:"kind"`
	RevisionID        string  `json:"revisionId"`
	RoutingGeneration string  `json:"routingGeneration"`
	Assessment        string  `json:"assessment"`
	Reason            string  `json:"reason"`
	ResourceCount     int     `json:"resourceCount"`
	Blocker           string  `json:"blocker"`
	FirstSeenAt       string  `json:"firstSeenAt"`
	LastSeenAt        string  `json:"lastSeenAt"`
	SourceCreatedAt   *string `json:"sourceCreatedAt"`
	SourceUpdatedAt   *string `json:"sourceUpdatedAt"`
}

type closureTimeItem struct {
	Kind       string  `json:"kind"`
	RevisionID string  `json:"revisionId"`
	Slot       int     `json:"slot"`
	Type       *string `json:"type"`
	Value      *string `json:"value"`
}

type closureResourceItem struct {
	Kind          string `json:"kind"`
	RevisionID    string `json:"revisionId"`
	Index         int    `json:"index"`
	ResourceID    string `json:"resourceId"`
	Region        string `json:"region"`
	Disposition   string `json:"disposition"`
	Reason        string `json:"reason"`
	DestinationID string `json:"destinationId"`
	BatchID       string `json:"batchId"`
}

func closureRevisionItems(record noticeledger.Record, revisions []noticeledger.Revision, request command) []any {
	items := []any{}
	for _, v := range revisions {
		blocker, _ := record.ClosureBlocker(noticeledger.CloseRevision, v.Digest)
		items = append(items, closureRevisionItem{"REVISION", v.Digest, v.RoutingGeneration, string(v.Assessment), string(v.AssessmentReason), len(v.Resources), blocker, v.FirstSeenAt, v.LastSeenAt, v.Source.TimeCreated, v.Source.TimeUpdated})
		for i, stamp := range v.Source.Times {
			items = append(items, closureTimeItem{"REVISION_TIME", v.Digest, i + 1, stamp.Type, stamp.Value})
		}
		if request.kind == "" {
			continue
		}
		for _, row := range v.Resources {
			if request.kind == string(noticeledger.CloseBatch) && row.BatchID != request.targetID {
				continue
			}
			items = append(items, closureResourceItem{"RESOURCE", v.Digest, row.Index, row.ResourceID, row.Region, row.Disposition, row.Reason, row.DestinationID, row.BatchID})
		}
	}
	return items
}
