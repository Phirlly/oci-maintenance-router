package reconciliation

import (
	"bytes"
	"context"
	"strings"
)

// Save accepts only an immediate proposal from this exact record/store handle.
// Conflict or uncertainty permits one exact confirmation, never retry or rebase.
func (s *BaselineSelectionStore) Save(ctx context.Context, previous StoredBaselineSelection, next BaselineSelection) (StoredBaselineSelection, error) {
	if !s.usable() || previous.store != s || next.data == nil || next.parent == nil || next.parent != previous.selection.data {
		return StoredBaselineSelection{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return StoredBaselineSelection{}, err
	}
	defer cancel()
	return s.write(ctx, previous.etag, next)
}
func (s *BaselineSelectionStore) write(ctx context.Context, previousTag string, next BaselineSelection) (StoredBaselineSelection, error) {
	body, _ := next.Encode()
	if ctx.Err() != nil {
		return StoredBaselineSelection{}, baselineStorageFailure(ctx, "write_failed", 0, false)
	}
	var tag string
	var err error
	if previousTag == "" {
		tag, err = s.objects.Create(ctx, baselineSelectionName, body)
	} else {
		tag, err = s.objects.Replace(ctx, baselineSelectionName, previousTag, body)
	}
	confirm, uncertain, status := false, true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	} else {
		confirm = !baselineETag(tag)
	}
	if ctx.Err() != nil || (err != nil && !confirm) {
		return StoredBaselineSelection{}, baselineStorageFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return StoredBaselineSelection{s, next, strings.Clone(tag)}, nil
	}
	got, _, failure := s.load(ctx)
	if failure != nil {
		failure.WriteUncertain = uncertain
		return StoredBaselineSelection{}, failure
	}
	return selectionConfirmed(ctx, got, next, uncertain)
}
func selectionConfirmed(ctx context.Context, got StoredBaselineSelection, next BaselineSelection, uncertain bool) (StoredBaselineSelection, error) {
	// Encode anew: a backend may have mutated the earlier request buffer.
	expected, _ := next.Encode()
	actual, _ := got.selection.Encode()
	if ctx.Err() != nil {
		return StoredBaselineSelection{}, baselineStorageFailure(ctx, "write_failed", 200, uncertain)
	}
	if !bytes.Equal(expected, actual) {
		return StoredBaselineSelection{}, &BaselineStorageError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
