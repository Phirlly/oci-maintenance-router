package deployment

import (
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/reconciliation"
	"oci-maintenance-router/internal/routing"
)

type recoveryRevisionItem struct {
	Kind          string `json:"kind"`
	RevisionID    string `json:"revisionId"`
	Assessment    string `json:"assessment"`
	Reason        string `json:"reason"`
	ResourceCount int    `json:"resourceCount"`
	FirstSeenAt   string `json:"firstSeenAt"`
	LastSeenAt    string `json:"lastSeenAt"`
}

type recoveryRouteItem struct {
	Kind          string `json:"kind"`
	ResourceID    string `json:"resourceId"`
	Region        string `json:"region"`
	Disposition   string `json:"disposition"`
	Reason        string `json:"reason"`
	DestinationID string `json:"destinationId"`
	Eligible      bool   `json:"eligible"`
}

func recoveryReviewItems(record noticeledger.Record, view reconciliation.RecoveryReview, request command) ([]any, error) {
	s := record.Snapshot()
	items := []any{closureNoticeItem{"SAVED_NOTICE", s.NoticeID, string(s.HoldReason), string(s.SourceOutcome.State), s.SourceOutcome.RevisionDigest, s.SourceOutcome.HTTPStatus, s.SourceOutcome.RecordedAt}}
	revision := request.targetID
	batchMembers := map[string]bool{}
	if request.kind == "RETRY_BATCH" {
		for _, b := range s.Batches {
			if b.Message.BatchID == request.targetID {
				revision = b.Message.Revision.Digest
				for _, row := range b.Message.Resources {
					batchMembers[row.ResourceID] = true
				}
			}
		}
	}
	excluded := map[string]bool{}
	for _, v := range s.Revisions {
		if request.kind != "" && v.Digest != revision {
			continue
		}
		items = append(items, recoveryRevisionItem{"SAVED_REVISION", v.Digest, string(v.Assessment), string(v.AssessmentReason), len(v.Resources), v.FirstSeenAt, v.LastSeenAt})
		for _, row := range v.Resources {
			excluded[row.ResourceID] = row.BatchID != "" || row.Disposition == "CLOSED"
		}
		if request.kind != "" {
			for i, stamp := range v.Source.Times {
				items = append(items, closureTimeItem{"SOURCE_TIME", v.Digest, i + 1, stamp.Type, stamp.Value})
			}
		}
	}
	for _, b := range s.Batches {
		if request.kind == "RETRY_BATCH" && b.Message.BatchID != request.targetID || request.kind == "APPROVE_REVISION" && b.Message.Revision.Digest != revision {
			continue
		}
		row := closureBatch(record, b)
		// This is a saved-state catalog, not the closure eligibility decision.
		row.Kind, row.Blocker = "SAVED_BATCH", ""
		items = append(items, row)
	}
	items = append(items, closureHistory(s.Recovery)...)
	if request.kind == "" {
		return items, nil
	}
	for _, member := range view.Component.Members {
		items = append(items, map[string]string{"kind": "COMPONENT_MEMBER", "noticeId": member.Revision.NoticeID, "revisionId": member.Revision.Digest})
	}
	for _, d := range view.Routing.Decisions {
		if request.kind == "RETRY_BATCH" && !batchMembers[d.ResourceID] {
			continue
		}
		eligible := d.Disposition == routing.Ready && (request.kind == "RETRY_BATCH" || !excluded[d.ResourceID])
		items = append(items, recoveryRouteItem{"CURRENT_ROUTING", d.ResourceID, d.Region, string(d.Disposition), string(d.Reason), d.DestinationID, eligible})
	}
	if view.Blocker != "" {
		return append(items, map[string]string{"kind": "BLOCKER", "code": view.Blocker}), nil
	}
	if view.Fingerprint == "" {
		return nil, operationFailure("recovery_review_invalid")
	}
	code, err := recoveryReviewCode(view, request)
	if err != nil {
		return nil, err
	}
	return append(items, code), nil
}
