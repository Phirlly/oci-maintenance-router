package deployment

import (
	"context"
	"encoding/json"

	"oci-maintenance-router/internal/configuration"
	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ociannouncements"
	"oci-maintenance-router/internal/ocistorage"
	"oci-maintenance-router/internal/reconciliation"
)

func composeRecoveryReview(s Settings, store Store, objects operationalObjects, p pinnedInputs) (operations, error) {
	saved, err := ocistorage.NewNoticeStore(objects, p.environment)
	if err != nil {
		return operations{}, err
	}
	return operations{recoveryReview: func(ctx context.Context, request command) (*RecoveryReviewPage, error) {
		var view reconciliation.RecoveryReview
		if request.kind == "" {
			record, err := saved.Load(ctx, request.noticeID)
			if err != nil {
				return nil, operationFailure("recovery_notice_read")
			}
			view.Saved = record.Record()
		} else {
			managed, ok := store.(*managedStore)
			if !ok || managed.principal == nil {
				return nil, operationFailure("managed_service_binding")
			}
			source, err := ociannouncements.New(s.Region, s.TenancyID, managed.principal)
			if err != nil {
				return nil, operationFailure("announcement_configuration")
			}
			clients, err := composeOwnership(p.config, s.TenancyID, managed.principal)
			if err != nil {
				return nil, operationFailure("ownership_configuration")
			}
			reviewer, err := reconciliation.NewRecoveryReviewer(p.configBytes, p.catalogBytes, configuration.LedgerLocation{Namespace: s.Namespace, Bucket: s.Bucket}, source, saved, clients)
			if err != nil {
				return nil, operationFailure("recovery_review_configuration")
			}
			view, err = reviewer.Review(ctx, request.noticeID, noticeledger.ActionKind(request.kind), request.targetID)
			if err != nil {
				return nil, err
			}
		}
		return projectRecoveryReview(s, view, request, p.config.SchemaVersion == 9)
	}}, nil
}

func projectRecoveryReview(s Settings, view reconciliation.RecoveryReview, request command, announcement bool) (*RecoveryReviewPage, error) {
	// The same in-memory legacy migration supplies saved context; no closure
	// eligibility or projected observation timestamp is presented as saved work.
	record, err := projectClosureRecord(view.Saved, announcement)
	if err != nil {
		return nil, operationFailure("recovery_projection_failed")
	}
	items, err := recoveryReviewItems(record, view, request)
	if err != nil {
		return nil, err
	}
	savedBytes, _ := view.Saved.Encode()
	projectedBytes, _ := view.Projected.Encode()
	binding, _ := json.Marshal([]any{"oci-maintenance-router/recovery-review", 1, digest(savedBytes), digest(projectedBytes), view.Routing, view.Component, view.Blocker, s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256, request.kind, request.targetID})
	return recoveryReviewPage(view.Saved.Snapshot(), items, digest(binding), request)
}
