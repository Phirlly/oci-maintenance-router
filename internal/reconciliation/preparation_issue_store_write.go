package reconciliation

import (
	"context"
	"strings"

	"oci-maintenance-router/internal/noticeledger"
	"oci-maintenance-router/internal/ocistorage"
)

// Save never reloads, rebases or retries. Only the coordinator can guard a later
// exact confirmation under its original authority/lifetime after uncertainty.
func (s *PreparationIssueStore) Save(ctx context.Context, previous StoredPreparationIssues, next noticeledger.PreparationIssues) (StoredPreparationIssues, error) {
	if !s.usable() || previous.store != s || !next.Follows(previous.record) || !next.Matches(previous.notice) || (!previous.absent && !baselineETag(previous.etag)) {
		return StoredPreparationIssues{}, &PreparationIssueStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := preparationIssueContext(ctx)
	if err != nil {
		return StoredPreparationIssues{}, err
	}
	defer cancel()
	body, _ := next.Encode()
	if ctx.Err() != nil {
		return StoredPreparationIssues{}, preparationIssueFailure(ctx, "write_failed", 0, false)
	}
	var tag string
	if previous.absent {
		tag, err = s.objects.Create(ctx, preparationIssueName(previous.notice), body)
	} else {
		tag, err = s.objects.Replace(ctx, preparationIssueName(previous.notice), previous.etag, body)
	}
	if err != nil {
		code, status, uncertain := "write_failed", baselineStorageStatus(err), true
		if e, ok := err.(*ocistorage.Error); ok && e != nil {
			uncertain = e.WriteUncertain
			if e.Code == ocistorage.Conflict {
				code, uncertain = "conflict", false
			} else if e.Code == ocistorage.Service && status >= 400 && status < 500 {
				uncertain = false
			} else if confirm, _ := baselineWriteDisposition(err); confirm {
				code = "write_uncertain"
			}
		}
		return StoredPreparationIssues{}, preparationIssueFailure(ctx, code, status, uncertain)
	}
	if ctx.Err() != nil {
		return StoredPreparationIssues{}, preparationIssueFailure(ctx, "write_failed", 200, true)
	}
	if !baselineETag(tag) {
		return StoredPreparationIssues{}, &PreparationIssueStorageError{Code: "write_uncertain", HTTPStatus: 200, WriteUncertain: true}
	}
	return StoredPreparationIssues{store: s, notice: previous.notice, record: next, etag: strings.Clone(tag)}, nil
}
