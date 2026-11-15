package deployment

import (
	"context"
	"encoding/json"
	"time"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

func composeClosureReview(s Settings, objects operationalObjects, p pinnedInputs) (operations, error) {
	store, err := ocistorage.NewNoticeStore(objects, p.environment)
	if err != nil {
		return operations{}, err
	}
	return operations{closureReview: func(ctx context.Context, request command) (*ClosureReviewPage, error) {
		saved, err := store.Load(ctx, request.noticeID)
		if err != nil {
			return nil, operationFailure("closure_notice_read")
		}
		original := saved.Record()
		projected, err := projectClosureRecord(original)
		if err != nil {
			return nil, operationFailure("closure_projection_failed")
		}
		items, err := closureItems(projected, p.generation, request)
		if err != nil {
			return nil, err
		}
		body, _ := original.Encode()
		binding, _ := json.Marshal([]any{"oci-maintenance-router/closure-review", 1, p.environment,
			digest(body), s.ConfigurationSHA256, s.CatalogSHA256, s.ReleaseSHA256, request.kind, request.targetID})
		return closurePage(original.Snapshot(), items, digest(binding), request)
	}}, nil
}

func projectClosureRecord(record noticeledger.Record) (noticeledger.Record, error) {
	saved := record.Snapshot()
	at, err := time.Parse(time.RFC3339Nano, saved.UpdatedAt)
	if err != nil {
		return noticeledger.Record{}, err
	}
	if saved.SchemaVersion == 1 {
		record, err = record.MigrateSourceOutcome(at)
		if err != nil {
			return noticeledger.Record{}, err
		}
	}
	if record.Snapshot().SchemaVersion == 2 {
		return record.MigrateRecovery(at)
	}
	return record, nil
}
