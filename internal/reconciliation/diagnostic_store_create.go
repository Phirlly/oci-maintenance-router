package reconciliation

import "context"

// Create preserves the first same-intent record, even when another operation
// claimed it or it has progressed. The caller must not infer send ownership from
// success. Ordinary deletion cannot reset a request while its history remains.
func (s *DiagnosticStore) Create(ctx context.Context, initial DiagnosticRecord) (StoredDiagnostic, error) {
	if !s.usable() || initial.data == nil || initial.parent != nil || initial.data.State != DiagnosticSending || initial.data.Intent.EnvironmentID != s.environment || initial.data.Intent.InstallationID != s.installation {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "invalid_request"}
	}
	ctx, cancel, err := diagnosticStoreContext(ctx)
	if err != nil {
		return StoredDiagnostic{}, err
	}
	defer cancel()
	request := initial.data.Intent.RequestID
	got, missing, failure := s.load(ctx, request)
	if failure == nil {
		return diagnosticExisting(ctx, got, initial)
	}
	if !missing {
		return StoredDiagnostic{}, failure
	}
	if ctx.Err() != nil {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "history_read", 0, false)
	}
	empty, err := s.objects.HistoryIsEmpty(ctx, s.name(request))
	if err != nil || ctx.Err() != nil {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "history_read", baselineStorageStatus(err), false)
	}
	if !empty {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "history_retained"}
	}
	return s.write(ctx, "", initial)
}
func diagnosticExisting(ctx context.Context, got StoredDiagnostic, initial DiagnosticRecord) (StoredDiagnostic, error) {
	if ctx.Err() != nil {
		return StoredDiagnostic{}, diagnosticStorageFailure(ctx, "read_failed", 200, false)
	}
	if !got.record.Matches(initial.data.Intent) {
		return StoredDiagnostic{}, &DiagnosticStorageError{Code: "identity_conflict", HTTPStatus: 200}
	}
	return got, nil
}
