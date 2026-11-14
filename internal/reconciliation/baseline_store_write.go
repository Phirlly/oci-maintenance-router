package reconciliation

import (
	"bytes"
	"context"
)

// Create makes one conditional write. A conflict or uncertain response permits
// only one exact read-back within the same lifetime, never a replacement/retry.
func (s *BaselineStore) Create(ctx context.Context, b Baseline) (Baseline, error) {
	if !s.usable() || b.data == nil || b.EnvironmentID() != s.environmentID {
		return Baseline{}, &BaselineStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := baselineStoreContext(ctx)
	if err != nil {
		return Baseline{}, err
	}
	defer cancel()
	body, _ := b.Encode()
	if ctx.Err() != nil {
		return Baseline{}, baselineStorageFailure(ctx, "write_failed", 0, false)
	}
	tag, err := s.objects.Create(ctx, "baselines/"+b.ID()+".json", body)
	confirm, uncertain, status := false, true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	} else {
		confirm = !baselineETag(tag)
	}
	if ctx.Err() != nil || (err != nil && !confirm) {
		return Baseline{}, baselineStorageFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return b, nil
	}
	got, failure := s.load(ctx, b.ID())
	if failure != nil {
		failure.WriteUncertain = uncertain
		return Baseline{}, failure
	}
	// Compare with the immutable proposal, not the buffer supplied to a backend.
	expected, _ := b.Encode()
	actual, _ := got.Encode()
	if ctx.Err() != nil {
		return Baseline{}, baselineStorageFailure(ctx, "write_failed", 200, uncertain)
	}
	if !bytes.Equal(expected, actual) {
		return Baseline{}, &BaselineStorageError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
