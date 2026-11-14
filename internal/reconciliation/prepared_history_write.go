package reconciliation

import (
	"bytes"
	"context"
	"strings"
)

// Create only conditionally writes an empty head. The initializer must first
// establish pinned PREVIEW intent, history absence, CURRENT controls and lease.
// Confirmation preserves an already-progressed same-installation head.
func (s *PreparedHistoryStore) Create(ctx context.Context) (StoredPreparedHistory, error) {
	if !s.usable() {
		return StoredPreparedHistory{}, &BaselineStorageError{Code: "invalid_request"}
	}
	h, _ := NewPreparedHistory(s.environment, s.installation)
	return s.write(ctx, "", h)
}

// Save accepts exactly one transition of the record bound to previous's ETag.
// It cannot combine a proposal derived from older state with a fresh version.
func (s *PreparedHistoryStore) Save(ctx context.Context, previous StoredPreparedHistory, next PreparedHistory) (StoredPreparedHistory, error) {
	if !s.usable() || previous.store != s || next.data == nil || next.parent == nil || next.parent != previous.history.data {
		return StoredPreparedHistory{}, &BaselineStorageError{Code: "invalid_request"}
	}
	return s.write(ctx, previous.etag, next)
}
func (s *PreparedHistoryStore) write(ctx context.Context, previousTag string, next PreparedHistory) (StoredPreparedHistory, error) {
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return StoredPreparedHistory{}, err
	}
	defer cancel()
	body, _ := next.Encode()
	if ctx.Err() != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "write_failed", 0, false)
	}
	var tag string
	if previousTag == "" {
		tag, err = s.objects.Create(ctx, preparedHistoryName, body)
	} else {
		tag, err = s.objects.Replace(ctx, preparedHistoryName, previousTag, body)
	}
	confirm, uncertain, status := false, true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	} else {
		confirm = !baselineETag(tag)
	}
	if ctx.Err() != nil || (err != nil && !confirm) {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return StoredPreparedHistory{s, next, strings.Clone(tag)}, nil
	}
	got, failure := s.load(ctx)
	if failure != nil {
		failure.WriteUncertain = uncertain
		return StoredPreparedHistory{}, failure
	}
	expected, _ := next.Encode()
	actual, _ := got.history.Encode()
	if ctx.Err() != nil {
		return StoredPreparedHistory{}, baselineStorageFailure(ctx, "write_failed", 200, uncertain)
	}
	if previousTag != "" && !bytes.Equal(expected, actual) {
		return StoredPreparedHistory{}, &BaselineStorageError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
