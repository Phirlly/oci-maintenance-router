package deployment

import (
	"context"
	"encoding/json"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/reconciliation"
)

// This composition has no service producers. The outer worker owns lease and
// CURRENT checks; inspection never migrates or writes business records.
func composeSavedReview(s Settings, objects operationalObjects, p pinnedInputs) (operations, error) {
	notices, err := ocistorage.NewNoticeStore(objects, p.environment)
	if err != nil {
		return operations{}, err
	}
	issues, err := reconciliation.NewPreparationIssueStore(objects, p.environment)
	if err != nil {
		return operations{}, err
	}
	return operations{review: func(ctx context.Context, request command) (*SavedReviewPage, error) {
		saved, err := notices.Load(ctx, request.noticeID)
		if err != nil {
			return nil, operationFailure("review_notice_read")
		}
		retained, err := issues.Load(ctx, saved.Record())
		if err != nil {
			return nil, operationFailure("review_issues_read")
		}
		view, err := saved.Record().Review(retained.Record(), s.ConfigurationSHA256)
		if err != nil {
			return nil, operationFailure("review_invalid")
		}
		body, _ := saved.Record().Encode() // Load returned a validated canonical record.
		binding, _ := json.Marshal([]any{"oci-maintenance-router/saved-review", 1, p.environment,
			digest(body), retained.Record().Snapshot(), s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256})
		return savedReviewPage(saved.Record().Snapshot(), view, digest(binding), request.cursor)
	}}, nil
}

func savedReviewItems(view noticeledger.SavedReview) []SavedReviewItem {
	items := make([]SavedReviewItem, 0, len(view.Issues)+len(view.Preparation))
	for _, item := range view.Issues {
		items = append(items, SavedReviewItem{Kind: item.Kind, Code: item.Code,
			RevisionDigest: item.RevisionDigest, BatchID: item.BatchID, ResourceIndex: item.ResourceIndex})
	}
	for _, item := range view.Preparation {
		v := item.Observation
		items = append(items, SavedReviewItem{Kind: "PREPARATION", Code: v.Code,
			RevisionDigest: v.RevisionDigest, ResourceIndex: v.ResourceIndex, Status: item.Status,
			LastSeenAt: v.LastSeenAt, DestinationID: v.DestinationID})
	}
	return items
}
