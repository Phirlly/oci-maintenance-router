package reconciliation

import (
	"context"
	"strings"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

// Load returns a bound absent handle only after both current absence and empty
// retained history. An unreadable/deleted object is never silently reinitialized.
func (s *PreparationIssueStore) Load(ctx context.Context, r noticeledger.Record) (StoredPreparationIssues, error) {
	env, id := r.Identity()
	if !s.usable() || env != s.environment || id == "" {
		return StoredPreparationIssues{}, &PreparationIssueStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := preparationIssueContext(ctx)
	if err != nil {
		return StoredPreparationIssues{}, err
	}
	defer cancel()
	object, err := s.objects.Get(ctx, preparationIssueName(r))
	if err != nil || ctx.Err() != nil {
		e, typed := err.(*ocistorage.Error)
		if ctx.Err() == nil && typed && e != nil && e.Code == ocistorage.Service && e.Status == 404 {
			empty, failure := s.objects.HistoryIsEmpty(ctx, preparationIssueName(r))
			if failure != nil || ctx.Err() != nil {
				return StoredPreparationIssues{}, preparationIssueFailure(ctx, "history_read", baselineStorageStatus(failure), false)
			}
			if !empty {
				return StoredPreparationIssues{}, &PreparationIssueStorageError{Code: "history_retained"}
			}
			return StoredPreparationIssues{store: s, notice: r, absent: true}, nil
		}
		return StoredPreparationIssues{}, preparationIssueFailure(ctx, "read_failed", baselineStorageStatus(err), false)
	}
	if !baselineETag(object.ETag) {
		return StoredPreparationIssues{}, &PreparationIssueStorageError{Code: "invalid_response", HTTPStatus: 200}
	}
	record, err := noticeledger.DecodePreparationIssues(object.Body, r)
	if err != nil || ctx.Err() != nil {
		return StoredPreparationIssues{}, preparationIssueFailure(ctx, "invalid_record", 200, false)
	}
	return StoredPreparationIssues{store: s, notice: r, record: record, etag: strings.Clone(object.ETag)}, nil
}
