package reconciliation

import (
	"bytes"
	"context"
	"strings"
)

// Save only accepts the immediate transition of the exact saved handle. It
// cannot attach an old or decoded proposal to another record's fresh ETag.
func (s *DiagnosticStore) Save(ctx context.Context, previous StoredDiagnostic, next DiagnosticRecord) (StoredDiagnostic, error) {
	if !s.usable() || previous.store != s || next.data == nil || next.parent == nil || next.parent != previous.record.data {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := diagnosticStoreContext(ctx)
	if err != nil {
		return StoredDiagnostic{}, err
	}
	defer cancel()
	return s.write(ctx, previous.etag, next)
}
func (s *DiagnosticStore) write(ctx context.Context, previousTag string, next DiagnosticRecord) (StoredDiagnostic, error) {
	body, _ := next.Encode()
	if ctx.Err() != nil {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "write_failed", 0, false)
	}
	name := s.name(next.data.Intent.RequestID)
	var tag string
	var err error
	if previousTag == "" {
		tag, err = s.objects.Create(ctx, name, body)
	} else {
		tag, err = s.objects.Replace(ctx, name, previousTag, body)
	}
	confirm, uncertain, status := false, true, 200
	if err != nil {
		confirm, uncertain = baselineWriteDisposition(err)
		status = baselineStorageStatus(err)
	} else {
		confirm = !baselineETag(tag)
	}
	if ctx.Err() != nil || (err != nil && !confirm) {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "write_failed", status, uncertain)
	}
	if !confirm {
		return StoredDiagnostic{s, next, strings.Clone(tag)}, nil
	}
	got, _, failure := s.load(ctx, next.data.Intent.RequestID)
	if failure != nil {
		failure.WriteUncertain = uncertain
		return StoredDiagnostic{}, failure
	}
	if previousTag == "" {
		result, err := diagnosticExisting(ctx, got, next)
		if err != nil {
			err.(*DiagnosticStorageError).WriteUncertain = uncertain
		}
		return result, err
	}
	expected, _ := next.Encode()
	actual, _ := got.record.Encode()
	if ctx.Err() != nil {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "write_failed", 200, uncertain)
	}
	if !bytes.Equal(expected, actual) {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "identity_conflict", HTTPStatus: 200, WriteUncertain: uncertain}
	}
	return got, nil
}
